package runtime

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/env"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/runtime"
	"github.com/refunc/refunc/pkg/runtime/types"
	"github.com/refunc/refunc/pkg/transport"
	"github.com/refunc/refunc/pkg/utils"
	"github.com/refunc/refunc/pkg/utils/rfutil"
	"github.com/refunc/refunc/pkg/version"
)

type lambda struct {
}

var _ runtime.Interface = (*lambda)(nil)

func (rt *lambda) Name() string {
	return "lambda"
}

// IsPodReady checks if the given pod is runnable
func (rt *lambda) IsPodReady(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	if len(pod.Status.ContainerStatuses) == 0 {
		return false
	}
	cs := pod.Status.ContainerStatuses[0]
	return cs.State.Running != nil
}

// GetDeploymentTemplate returns a deployment of the runner
func (rt *lambda) GetDeploymentTemplate(tpl *rfv1beta3.Xenv) *v1beta1.Deployment {
	var replicas = defaultPoolSize
	if tpl.Spec.PoolSize != 0 {
		replicas = int32(tpl.Spec.PoolSize)
	}
	var extraCfg struct {
		NoInject bool `json:"noInject,omitempty"`
	}
	json.Unmarshal(tpl.Spec.Extra, &extraCfg) // error is fine

	// setting up containers
	container := tpl.Spec.Container.DeepCopy()
	container.Name = "body"

	var initContainers []corev1.Container
	if !extraCfg.NoInject {
		orginCmd := container.Command
		// overrride original command wiht dinit, block and wait
		container.Command = []string{pathInVolume("loader"), "--v", "3"}
		if len(orginCmd) > 0 {
			container.Command = append(container.Command, "--")
			container.Command = append(container.Command, orginCmd...)
		}
		initContainers = append(initContainers, *initContainer.DeepCopy())
	}

	// set volume mounts
	container.VolumeMounts = append(container.VolumeMounts, *(refuncVolumeMnt.DeepCopy()))

	// set environs
	container.Env = append(container.Env,
		// FIXME(bin)
		corev1.EnvVar{
			Name:  "REFUNC_APP",
			Value: "loader",
		},
	)

	if len(container.Resources.Limits) == 0 && len(container.Resources.Requests) == 0 {
		klog.V(4).Info("(loader) using default resources' limits and requests")
		container.Resources = *defaultResource.DeepCopy()
	}
	if len(container.Resources.Requests) == 0 {
		container.Resources.Requests = defaultResource.Requests.DeepCopy()
	} else {
		// hpa requires those fields to be set
		if _, has := container.Resources.Requests[corev1.ResourceCPU]; !has {
			container.Resources.Requests[corev1.ResourceCPU] = defaultResource.Requests[corev1.ResourceCPU].DeepCopy()
		}
		if _, has := container.Resources.Requests[corev1.ResourceMemory]; !has {
			container.Resources.Requests[corev1.ResourceMemory] = defaultResource.Requests[corev1.ResourceMemory].DeepCopy()
		}
	}
	if len(container.Resources.Limits) == 0 {
		container.Resources.Limits = defaultResource.Limits.DeepCopy()
	}

	// ensure container user is root
	if container.SecurityContext == nil {
		container.SecurityContext = new(corev1.SecurityContext)
	}
	var rootUID int64
	container.SecurityContext.RunAsUser = &rootUID

	var containers = []corev1.Container{*container}
	transp := transport.ForXenv(tpl)
	if sidecar := transp.GetTransportContainer(tpl); sidecar != nil {
		sidecar.VolumeMounts = append(sidecar.VolumeMounts, *(refuncVolumeMnt.DeepCopy()))
		containers = append(containers, *sidecar)
	}

	dep := &v1beta1.Deployment{
		Spec: v1beta1.DeploymentSpec{
			Replicas:             &replicas,
			Selector:             &metav1.LabelSelector{}, // MatchLabels will be filled by controller
			RevisionHistoryLimit: &historyLimits,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers:   initContainers,
					Containers:       containers,
					ImagePullSecrets: tpl.Spec.ImagePullSecrets[:],
					Volumes:          append([]corev1.Volume{*(refuncVolume.DeepCopy())}, tpl.Spec.Volumes...),
				},
			},
		},
	}
	if tpl.Spec.ServiceAccount != "" {
		dep.Spec.Template.Spec.ServiceAccountName = tpl.Spec.ServiceAccount
	}

	return dep
}

// InitPod initialize given pod
// Note: one should not assume that the workDir still persist after InitPod being called
func (rt *lambda) InitPod(pod *corev1.Pod, funcinst *rfv1beta3.Funcinst, fndef *rfv1beta3.Funcdef, xenv *rfv1beta3.Xenv, workDir string) error {
	name := rfutil.ExecutorPodName(pod)

	var t0 = time.Now()
	defer func() {
		// stat upon finish
		d2 := (time.Since(t0) / time.Millisecond) * time.Millisecond
		klog.Infof("(loader) %s| taking %v provisioning", name, d2)
	}()

	fn, err := rt.genFunction(pod, funcinst, fndef)
	if err != nil {
		return err
	}

	bts, err := json.Marshal(&fn)
	if err != nil {
		return err
	}

	var rsp *http.Response
	for i := 0; i < 1; i++ {
		rsp, err = http.Post(fmt.Sprintf("http://%s:7788/init", pod.Status.PodIP), "application/json", bytes.NewReader(bts))
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}
	defer rsp.Body.Close()

	if rsp.StatusCode >= 500 {
		bts, err := ioutil.ReadAll(rsp.Body)
		if err != nil {
			return err
		}
		// try decode error message
		var errMsg messages.ErrorMessage
		if err := json.Unmarshal(bts, &errMsg); err == nil {
			return errMsg
		}
		return err
	}

	if klog.V(3) {
		outputs, err := ioutil.ReadAll(rsp.Body)
		if err != nil {
			// 200 or 400 but read failed, log out err
			klog.Errorf("(lamdba) fail to read response, %v", err)
			return nil
		}

		scanner := utils.NewScanner(bytes.NewReader(outputs))
		for scanner.Scan() {
			if text := scanner.Text(); len(text) > 0 {
				klog.Infof("(loader) %s| %s", name, text)
			}
		}
	}

	return nil
}

var (
	// InitContainerImage default init container image
	// the InitContainerImage can be override by env REFUNC_LOADER_INIT_IMAGE
	InitContainerImage = "refunc/loader:$latest"
	iciCheckEnvOnce    sync.Once // init container check

	historyLimits int32 = 3
)

func initContainerImage() string {
	iciCheckEnvOnce.Do(func() {
		if ci := os.Getenv("REFUNC_LOADER_INIT_IMAGE"); ci != "" {
			InitContainerImage = ci
		}
		InitContainerImage = strings.Replace(InitContainerImage, "$latest", version.LoaderVersion, -1)
	})
	return InitContainerImage
}

const (
	initContainerName = "loader-inject"
	refuncVolumeName  = "refunc"
	refuncVolumePath  = "/var/run/refunc"
)

func pathInVolume(paths ...string) string {
	return filepath.Join(append([]string{refuncVolumePath}, paths...)...)
}

var (
	refuncVolume = &corev1.Volume{
		Name: refuncVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	refuncVolumeMnt = &corev1.VolumeMount{
		Name:      refuncVolumeName,
		MountPath: refuncVolumePath,
	}
	initContainer = corev1.Container{
		Name:            initContainerName,
		Image:           initContainerImage(),
		ImagePullPolicy: corev1.PullIfNotPresent,
		VolumeMounts:    []corev1.VolumeMount{*(refuncVolumeMnt.DeepCopy())},
		Command: []string{
			"sh",
			"-c",
			"cp /usr/bin/loader " + pathInVolume("/loader"),
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("10Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("5Mi"),
			},
		},
	}
	defaultResource = corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("2.7Gi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("256m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
	}
	defaultPoolSize int32 = 1 // default replcias
)

func (rt *lambda) genFunction(pod *corev1.Pod, fninst *rfv1beta3.Funcinst, fndef *rfv1beta3.Funcdef) (*types.Function, error) {
	if fndef.Spec.Entry == "" {
		return nil, errors.New("lambda: handler is empty")
	}

	var fn types.Function
	fn.Namespace = fndef.Namespace
	fn.Name = fndef.Name
	fn.Labels = fninst.Labels
	fn.Annotations = fninst.Annotations
	fn.Spec.Body = fndef.Spec.Body
	fn.Spec.Entry = fndef.Spec.Entry
	fn.Spec.Hash = fndef.Spec.Hash
	fn.Spec.MaxReplicas = fndef.Spec.MaxReplicas
	fn.Spec.Runtime.Name = fndef.Spec.Runtime.Name
	fn.Spec.Runtime.Envs = fndef.Spec.Runtime.Envs
	fn.Spec.Runtime.Timeout = fndef.Spec.Runtime.Timeout
	fn.Spec.Runtime.Credentials.AccessKey = fninst.Spec.Runtime.Credentials.AccessKey
	fn.Spec.Runtime.Credentials.SecretKey = fninst.Spec.Runtime.Credentials.SecretKey
	fn.Spec.Runtime.Credentials.Token = fninst.Spec.Runtime.Credentials.Token
	fn.Spec.Runtime.Permissions.Scope = fninst.Spec.Runtime.Permissions.Scope
	fn.Spec.Runtime.Permissions.Publish = fninst.Spec.Runtime.Permissions.Publish
	fn.Spec.Runtime.Permissions.Subscribe = fninst.Spec.Runtime.Permissions.Subscribe

	// override system environments
	if len(fn.Spec.Runtime.Envs) == 0 {
		fn.Spec.Runtime.Envs = make(map[string]string)
	}
	fn.Spec.Runtime.Envs["REFUNC_MINIO_ENDPOINT"] = env.GlobalMinioEndpoint
	fn.Spec.Runtime.Envs["REFUNC_MINIO_PUBLIC_ENDPOINT"] = env.GlobalMinioPublicEndpoint
	fn.Spec.Runtime.Envs["REFUNC_MINIO_BUCKET"] = env.GlobalBucket
	fn.Spec.Runtime.Envs["REFUNC_NATS_ENDPOINT"] = env.GlobalNATSEndpoint
	// nats endpoints
	fn.Spec.Runtime.Envs["REFUNC_CRY_ENDPOINT"] = fninst.CryingEndpoint()
	fn.Spec.Runtime.Envs["REFUNC_TAP_ENDPOINT"] = fninst.TappingEndpoint()
	fn.Spec.Runtime.Envs["REFUNC_LOG_ENDPOINT"] = fninst.LoggingEndpoint()
	fn.Spec.Runtime.Envs["REFUNC_SVC_ENDPOINT"] = fninst.ServiceEndpoint()
	fn.Spec.Runtime.Envs["REFUNC_CRY_SVC_ENDPOINT"] = fninst.CryServiceEndpoint()

	// lambda
	fn.Spec.Runtime.Envs["AWS_LAMBDA_RUNTIME_API"] = "127.0.0.1:80"
	fn.Spec.Runtime.Envs["AWS_REGION"] = "us-east-1"
	fn.Spec.Runtime.Envs["AWS_DEFAULT_REGION"] = "us-east-1"
	fn.Spec.Runtime.Envs["AWS_ACCESS_KEY_ID"] = fninst.Spec.Runtime.Credentials.AccessKey
	fn.Spec.Runtime.Envs["AWS_SECRET_ACCESS_KEY"] = fninst.Spec.Runtime.Credentials.SecretKey
	fn.Spec.Runtime.Envs["AWS_SESSION_TOKEN"] = fninst.Spec.Runtime.Credentials.Token
	fn.Spec.Runtime.Envs["AWS_LAMBDA_FUNCTION_NAME"] = fndef.Name
	fn.Spec.Runtime.Envs["AWS_LAMBDA_FUNCTION_VERSION"] = fndef.Spec.Hash
	fn.Spec.Runtime.Envs["AWS_LAMBDA_LOG_GROUP_NAME"] = "/aws/lambda/" + fndef.Name
	fn.Spec.Runtime.Envs["AWS_LAMBDA_LOG_STREAM_NAME"] = logStreamName(fndef.Spec.Hash)
	// parse memory size
	mem, ok := pod.Spec.Containers[0].Resources.Limits.Memory().AsInt64()
	if !ok {
		mem = 1536
	} else {
		mem = mem / 1024 / 1024
	}
	fn.Spec.Runtime.Envs["AWS_LAMBDA_FUNCTION_MEMORY_SIZE"] = strconv.FormatInt(mem, 10)
	fn.Spec.Runtime.Envs["AWS_LAMBDA_FUNCTION_TIMEOUT"] = strconv.FormatInt(int64(fndef.Spec.Runtime.Timeout), 10)
	// TODO(bin) can we provide a meaningful?
	fn.Spec.Runtime.Envs["AWS_ACCOUNT_ID"] = strconv.FormatInt(int64(rand.Int31()), 10)
	fn.Spec.Runtime.Envs["AWS_LAMBDA_CLIENT_CONTEXT"] = ""
	fn.Spec.Runtime.Envs["AWS_LAMBDA_COGNITO_IDENTITY"] = ""
	fn.Spec.Runtime.Envs["_X_AMZN_TRACE_ID"] = ""
	fn.Spec.Runtime.Envs["_HANDLER"] = fn.Spec.Entry

	// utils for tensorflow
	fn.Spec.Runtime.Envs["S3_ENDPOINT"] = env.GlobalMinioPublicEndpoint
	if strings.HasPrefix(env.GlobalMinioPublicEndpoint, "https://") {
		fn.Spec.Runtime.Envs["S3_USE_HTTPS"] = "1"
	} else {
		fn.Spec.Runtime.Envs["S3_USE_HTTPS"] = "0"
	}
	if fninst.Spec.Runtime.Permissions.Scope != "" {
		fn.Spec.Runtime.Envs["S3_PREFIX"] = fmt.Sprintf("s3://%s/%s", env.GlobalBucket, strings.TrimSuffix(fninst.Spec.Runtime.Permissions.Scope, "/"))
	}

	if strings.HasPrefix(fn.Spec.Body, "s3://") || strings.HasPrefix(fn.Spec.Body, "minio://") {
		signedURL, err := genDownloadURL(fn.Spec.Body, time.Now().Add(60*time.Second))
		if err != nil {
			return nil, err
		}
		fn.Spec.Body = signedURL
	}

	return &fn, nil
}

// copied from https://github.com/lambci/docker-lambda/blob/master/go1.x/run/aws-lambda-mock.go#L251:6
func logStreamName(version string) string {
	randBuf := make([]byte, 16)
	rand.Read(randBuf)

	hexBuf := make([]byte, hex.EncodedLen(len(randBuf)))
	hex.Encode(hexBuf, randBuf)

	return time.Now().Format("2006/01/02") + "/[" + version + "]" + string(hexBuf)
}

func init() {
	runtime.Register(&lambda{})
}
