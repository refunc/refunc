package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"golang.org/x/crypto/ssh"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/env"
	"github.com/refunc/refunc/pkg/runtime"
	"github.com/refunc/refunc/pkg/runtime/refunc/loader"
	"github.com/refunc/refunc/pkg/utils/rfutil"
	"github.com/refunc/refunc/pkg/utils/rtutil"
)

type v1 struct {
}

var _ runtime.Interface = (*v1)(nil)

func (br *v1) Name() string {
	return "agent"
}

// IsPodReady checks if the given pod is runnable
func (br *v1) IsPodReady(pod *corev1.Pod) bool {
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
func (br *v1) GetDeploymentTemplate(tpl *rfv1beta3.Xenv) *v1beta1.Deployment {

	var replicas = defaultPoolSize
	if tpl.Spec.PoolSize != 0 {
		replicas = int32(tpl.Spec.PoolSize)
	}
	// setting up containers
	container := tpl.Spec.Container.DeepCopy()
	orginCmd := container.Command

	setupKey := tpl.Spec.SetupKey
	if setupKey == "" {
		setupKey = defaultSetupKey()
	}
	// encrypt setup key
	setupKey = rtutil.EncryptKey(setupKey)

	container.Name = "funcbody"

	// overrride original command wiht dinit, block and wait
	container.Command = []string{
		pathInVolume("agent"),
		"--v", "3",
		"-b", refuncDataVolumePath,
		"-k", setupKey,
	}

	var volumes = []corev1.Volume{*refuncDataVolume}

	// merge volumes
	volumes = append(volumes, tpl.Spec.Volumes...)

	// set volume mounts
	container.VolumeMounts = append(container.VolumeMounts,
		corev1.VolumeMount{
			Name:      refuncDataVolumeName,
			MountPath: refuncDataVolumePath,
		},
	)

	container.Env = append(container.Env,
		// FIXME: remove this when rpc v2 finished
		corev1.EnvVar{
			Name:  "REFUNC_APP",
			Value: "loader",
		},
	)

	if len(container.Resources.Limits) == 0 && len(container.Resources.Requests) == 0 {
		klog.V(4).Info("(agent) using default resources' limits and requests")
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

	dep := &v1beta1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent", // this will be overried by controller
		},
		Spec: v1beta1.DeploymentSpec{
			Replicas:             &replicas,
			Selector:             &metav1.LabelSelector{}, // MatchLabels will be filled by controller
			RevisionHistoryLimit: &histroyLimits,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes:          volumes,
					Containers:       []corev1.Container{*container},
					InitContainers:   []corev1.Container{*initContainer.DeepCopy()},
					ImagePullSecrets: tpl.Spec.ImagePullSecrets[:],
				},
			},
		},
	}
	if tpl.Spec.ServiceAccount != "" {
		dep.Spec.Template.Spec.ServiceAccountName = tpl.Spec.ServiceAccount
	}
	if len(orginCmd) > 0 {
		jbts, _ := json.Marshal(orginCmd)
		dep.Spec.Template.Annotations[runnerCmdAnnotaion] = string(jbts)
	}
	return dep
}

// InitPod initialize given pod
// Note: one should not assume that the workDir still persist after InitPod being called
func (br *v1) InitPod(pod *corev1.Pod, funcinst *rfv1beta3.Funcinst, fndef *rfv1beta3.Funcdef, xenv *rfv1beta3.Xenv, rcfg rest.Config, workDir string) error {
	name := rfutil.ExecutorPodName(pod)
	var (
		t0 = time.Now()
		d1 time.Duration
	)
	defer func() {
		// stat upon finish
		d1 = (d1 / time.Millisecond) * time.Millisecond
		d2 := (time.Since(t0) / time.Millisecond) * time.Millisecond
		klog.Infof("(agent) %s| taking %v, %v dialing, %v provisioning", name, d2, d1, d2-d1)
	}()

	usBuf := bytes.NewBuffer(nil)
	if err := br.genSetupScript(usBuf, pod, funcinst, fndef); err != nil {
		return err
	}

	setupKey := xenv.Spec.SetupKey
	if setupKey == "" {
		setupKey = defaultSetupKey()
	}

	client, err := ssh.Dial("tcp",
		pod.Status.PodIP+":7788",
		&ssh.ClientConfig{
			User:            "setup", // hard coded command
			Auth:            []ssh.AuthMethod{ssh.Password(setupKey)},
			HostKeyCallback: func(string, net.Addr, ssh.PublicKey) error { return nil },
		},
	)
	d1 = time.Since(t0)

	if err != nil {
		return err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	// send bootstrap script through stdin
	session.Stdin = usBuf

	// logging output to a temp file, show only on error
	logfile, err := ioutil.TempFile(workDir, "")
	if err != nil {
		return err
	}
	defer logfile.Close()
	output := &singleWriter{
		b: bufio.NewWriter(logfile),
	}
	session.Stderr = output
	session.Stdout = output
	printLogs := func() {
		output.b.(interface {
			Flush() error
		}).Flush()
		if _, err := logfile.Seek(0, 0); err == nil {
			br.scanOutput(name, logfile)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second) // timeout twice as long as in agent
	defer cancel()

	var execErr error
	go func() {
		defer cancel()
		execErr = session.Run("")
	}()

	// wait till comand finished
	<-ctx.Done()
	if ctx.Err() == context.DeadlineExceeded {
		session.Signal(ssh.SIGKILL) // exit and kill
		printLogs()
		return ctx.Err()
	}
	if execErr != nil {
		printLogs()
	}

	return execErr
}

type singleWriter struct {
	b  io.Writer
	mu sync.Mutex
}

func (w *singleWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (br *v1) scanOutput(prefix string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	raw := [4 * 1024]byte{}
	scanner.Buffer(raw[:], 16<<20)

	for scanner.Scan() {
		klog.Infof("(agent) %s| %s", prefix, scanner.Text())
	}
}

var (
	// InitContainerImage default init container image
	// the InitContainerImage can be override by env REFUNC_AGENT_INIT_CONTAINER
	InitContainerImage = "refunc/agent:latest"
	iciCheckEnvOnce    sync.Once // init container check
)

func initContainerImage() string {
	iciCheckEnvOnce.Do(func() {
		if ci := os.Getenv("REFUNC_AGENT_INIT_CONTAINER"); ci != "" {
			InitContainerImage = ci
		}
	})
	return InitContainerImage
}

var (
	// DefaultSetupKey is the key to access pod when init pod
	// if template is not set the "key" in its config, this will be use as default
	// the DefaultSetupKey can be override by env REFUNC_AGNET_SETUP_KEY
	DefaultSetupKey = "CwP@Ee$kgp7)qt59PxHyzI"
	dskChcekEnvOnce sync.Once

	histroyLimits int32 = 3
)

func defaultSetupKey() string {
	dskChcekEnvOnce.Do(func() {
		if sk := os.Getenv("REFUNC_AGNET_SETUP_KEY"); sk != "" {
			DefaultSetupKey = sk
		}
	})
	return DefaultSetupKey
}

const (
	initContainerName    = "refunc-init"
	refuncDataVolumeName = "refunc-data"
	refuncDataVolumePath = "/" + refuncDataVolumeName
	runnerCmdAnnotaion   = "agent.refunc.io/cmd"
	refuncBundleName     = "bundle.tar.gz"
)

func pathInVolume(paths ...string) string {
	return filepath.Join(append([]string{refuncDataVolumePath}, paths...)...)
}

var (
	refuncDataVolume = &corev1.Volume{
		Name: refuncDataVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	initContainer = corev1.Container{
		Name:            initContainerName,
		Image:           initContainerImage(),
		ImagePullPolicy: corev1.PullIfNotPresent,
		VolumeMounts: []corev1.VolumeMount{
			corev1.VolumeMount{
				Name:      refuncDataVolumeName,
				MountPath: refuncDataVolumePath,
			},
		},
		Command: []string{
			"sh",
			"-c",
			"cp /usr/bin/agent " + refuncDataVolumePath + "/agent",
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("20Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("10Mi"),
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

	uploadScript = template.Must(template.New("upload").Parse(`#!/bin/bash
set -e
set -o errexit
set -o pipefail
WORK_DIR=${REFUNC_DIR:-$(pwd)}
ROOT_DIR=${REFUNC_ROOT_DIR:-${WORK_DIR}/root}
echo "working dir: ${WORK_DIR}, refunc dir: ${ROOT_DIR}"
rm -rf ${ROOT_DIR} 2>/dev/null || true
mkdir -p ${ROOT_DIR} 2>/dev/null || true
echo "creating refunc.json"
echo '{{.RefuncBase64}}' | base64 -d >${WORK_DIR}/refunc.json
{{- if .BundleURL }}
echo "unpacking refunc bundle"
curl -sSL '{{.BundleURL}}' | tar xzf - -C ${ROOT_DIR}
{{- else if .SrcFileName }}
{{-   if eq .SrcFileName "bundle.tar.gz" }}
echo '{{.SrcBase64}}' | base64 -d | tar xzf - -C ${ROOT_DIR}
{{-   else}}
echo '{{.SrcBase64}}' | base64 -d >${ROOT_DIR}/{{.SrcFileName}}
chmod u+x ${ROOT_DIR}/{{.SrcFileName}}
{{-   end}}
{{- end}}
echo "done"
`))
)

func (br *v1) genSetupScript(wr io.Writer, pod *corev1.Pod, fninst *rfv1beta3.Funcinst, fndef *rfv1beta3.Funcdef) error {
	var (
		// template context
		scriptTpl struct {
			RefuncBase64 string
			BundleURL    string

			SrcFileName string
			SrcBase64   string
		}
	)

	name := rfutil.ExecutorPodName(pod)

	var fnRt loader.FuncRuntime
	fnRt.Namespace = fndef.Namespace
	fnRt.Name = fndef.Name
	fnRt.Labels = fninst.Labels
	fnRt.Annotations = fninst.Annotations
	fnRt.Spec.Entry = fndef.Spec.Entry
	fnRt.Spec.Hash = fndef.Spec.Hash
	fnRt.Spec.MaxReplicas = fndef.Spec.MaxReplicas
	fnRt.Spec.Runtime.Name = fndef.Spec.Runtime.Name
	fnRt.Spec.Runtime.Envs = fndef.Spec.Runtime.Envs
	fnRt.Spec.Runtime.Timeout = fndef.Spec.Runtime.Timeout
	fnRt.Spec.Runtime.Credentials.AccessKey = fninst.Spec.Runtime.Credentials.AccessKey
	fnRt.Spec.Runtime.Credentials.SecretKey = fninst.Spec.Runtime.Credentials.SecretKey
	fnRt.Spec.Runtime.Credentials.Token = fninst.Spec.Runtime.Credentials.Token
	fnRt.Spec.Runtime.Permissions.Scope = fninst.Spec.Runtime.Permissions.Scope
	fnRt.Spec.Runtime.Permissions.Publish = fninst.Spec.Runtime.Permissions.Publish
	fnRt.Spec.Runtime.Permissions.Subscribe = fninst.Spec.Runtime.Permissions.Subscribe

	// override system environments
	if len(fnRt.Spec.Runtime.Envs) == 0 {
		fnRt.Spec.Runtime.Envs = make(map[string]string)
	}
	fnRt.Spec.Runtime.Envs["REFUNC_MINIO_ENDPOINT"] = env.GlobalMinioEndpoint
	fnRt.Spec.Runtime.Envs["REFUNC_MINIO_PUBLIC_ENDPOINT"] = env.GlobalMinioPublicEndpoint
	fnRt.Spec.Runtime.Envs["REFUNC_MINIO_BUCKET"] = env.GlobalBucket
	fnRt.Spec.Runtime.Envs["REFUNC_NATS_ENDPOINT"] = env.GlobalNATSEndpoint
	// nats endpoints
	fnRt.Spec.Runtime.Envs["REFUNC_CRY_ENDPOINT"] = fninst.CryingEndpoint()
	fnRt.Spec.Runtime.Envs["REFUNC_TAP_ENDPOINT"] = fninst.TappingEndpoint()
	fnRt.Spec.Runtime.Envs["REFUNC_LOG_ENDPOINT"] = fninst.LoggingEndpoint()
	fnRt.Spec.Runtime.Envs["REFUNC_SVC_ENDPOINT"] = fninst.ServiceEndpoint()
	fnRt.Spec.Runtime.Envs["REFUNC_CRY_SVC_ENDPOINT"] = fninst.CryServiceEndpoint()

	// utils for s3 clients to access
	fnRt.Spec.Runtime.Envs["AWS_REGION"] = "us-east-1"
	fnRt.Spec.Runtime.Envs["AWS_ACCESS_KEY_ID"] = fninst.Spec.Runtime.Credentials.AccessKey
	fnRt.Spec.Runtime.Envs["AWS_SECRET_ACCESS_KEY"] = fninst.Spec.Runtime.Credentials.SecretKey
	fnRt.Spec.Runtime.Envs["S3_ENDPOINT"] = env.GlobalMinioPublicEndpoint
	if strings.HasPrefix(env.GlobalMinioPublicEndpoint, "https://") {
		fnRt.Spec.Runtime.Envs["S3_USE_HTTPS"] = "1"
	} else {
		fnRt.Spec.Runtime.Envs["S3_USE_HTTPS"] = "0"
	}
	fnRt.Spec.Runtime.Envs["S3_PREFIX"] = fmt.Sprintf("s3://%s/%s", env.GlobalBucket, strings.TrimSuffix(fninst.Spec.Runtime.Permissions.Scope, "/"))

	// setup refunc sources, from object storage or inline
	u, err := url.Parse(fndef.Spec.Body)
	if err != nil {
		return err
	}
	switch strings.ToLower(u.Scheme) {
	case "s3", "oss":
		signedURL, err := genDownloadURL(fndef.Spec.Body, time.Now().Add(60*time.Second))
		if err != nil {
			return err
		}
		scriptTpl.BundleURL = signedURL
	case "inline", "src":
		scriptTpl.SrcFileName = u.Host
		if strings.HasSuffix(scriptTpl.SrcFileName, ".tar.gz") {
			scriptTpl.SrcFileName = refuncBundleName
		}
		scriptTpl.SrcBase64 = strings.Trim(u.Path, "/")
	case "http", "https":
		scriptTpl.BundleURL = fndef.Spec.Body
	}

	// check entry
	if fndef.Spec.Entry == "" {
		if scriptTpl.SrcFileName != "" && scriptTpl.SrcFileName != refuncBundleName {
			fnRt.Spec.Entry = scriptTpl.SrcFileName
		} else if cmdJSON, ok := pod.Annotations[runnerCmdAnnotaion]; ok {
			var cmds []string
			json.Unmarshal([]byte(cmdJSON), &cmds)
			fnRt.Spec.Entry = strings.Join(cmds, " ")
		} else {
			klog.Warningf("(agent) %s| do not have a entry", name)
		}
	}

	fnJSON, _ := json.Marshal(fnRt)
	scriptTpl.RefuncBase64 = base64.StdEncoding.EncodeToString(fnJSON)

	return uploadScript.Execute(wr, scriptTpl)
}

func init() {
	runtime.Register(&v1{})
}
