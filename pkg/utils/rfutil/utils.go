package rfutil

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	rfcliv1 "github.com/refunc/refunc/pkg/generated/clientset/versioned/typed/refunc/v1beta3"
)

// IsRefuncRes checks if the given object has refunc labels,
// and return its meta for future filtering
func IsRefuncRes(obj interface{}) (metav1.Object, bool) {
	if apiObj, ok := getObject(obj); ok {
		if _, ok := apiObj.GetLabels()[rfv1beta3.LabelResType]; ok {
			return apiObj, true
		}
	}
	return nil, false
}

// IsExecutorRes checks if a given k8s resource belongs to a executor
func IsExecutorRes(obj interface{}) (metav1.Object, bool) {
	if apiObj, ok := IsRefuncRes(obj); ok {
		if apiObj.GetLabels()[rfv1beta3.LabelResType] == "executor" {
			return apiObj, true
		}
	}
	return nil, false
}

// IsXenvRes checks if a given k8s resources belongs to an xenv
func IsXenvRes(obj interface{}) (metav1.Object, bool) {
	if apiObj, ok := IsRefuncRes(obj); ok {
		if apiObj.GetLabels()[rfv1beta3.LabelResType] == "xenv-pool" {
			return apiObj, true
		}
	}
	return nil, false
}

// K8sResNameForRefunc returns a valid k8s name fro funcdef
func K8sResNameForRefunc(refunc *rfv1beta3.Funcdef) string {
	return fmt.Sprintf("%s-%s", refunc.Name, shortHash(refunc.Spec.Hash))
}

// ExecutorLabels infers a set of labels for corresponding rs, pods
func ExecutorLabels(funcinst *rfv1beta3.Funcinst) map[string]string {
	return map[string]string{
		rfv1beta3.LabelResType: "executor",
		rfv1beta3.LabelName:    funcinst.Name,
		rfv1beta3.LabelHash:    funcinst.Labels[rfv1beta3.LabelHash],
		rfv1beta3.LabelUID:     string(funcinst.UID),
	}
}

// FuncinstLabels infers a set of labels for corresponding funcinst
func FuncinstLabels(fndef *rfv1beta3.Funcdef) map[string]string {
	labels := map[string]string{
		rfv1beta3.LabelResType:  "funcinst",
		rfv1beta3.LabelName:     fndef.Name,
		rfv1beta3.LabelHash:     GetHash(fndef),
		rfv1beta3.LabelSpecHash: GetSpecHash(fndef),
	}
	if name, ok := fndef.Labels[rfv1beta3.LabelLambdaName]; ok {
		labels[rfv1beta3.LabelLambdaName] = name
	}
	if version, ok := fndef.Labels[rfv1beta3.LabelLambdaVersion]; ok {
		labels[rfv1beta3.LabelLambdaVersion] = version
	}
	return labels
}

// FuncinstAnnotations infers a set of annotations for corresponding funcinst
func FuncinstAnnotations(fndef *rfv1beta3.Funcdef) map[string]string {
	annotations := map[string]string{}
	if concurrency, ok := fndef.Annotations[rfv1beta3.AnnotationLambdaConcurrency]; ok {
		annotations[rfv1beta3.AnnotationLambdaConcurrency] = concurrency
	}
	return annotations
}

func GetHash(fndef *rfv1beta3.Funcdef) string {
	if len(fndef.Spec.Hash) >= 63 {
		return fndef.Spec.Hash[:32]
	}
	return fndef.Spec.Hash
}

func GetSpecHash(fndef *rfv1beta3.Funcdef) string {
	fn := fndef.DeepCopy()
	annotations := fn.Annotations
	spec := fn.Spec
	spec.Hash = ""
	return getMD5Hash(map[string]interface{}{
		"spec":        spec,
		"annotations": annotations,
	})
}

func GetFunctionVersion(fndef *rfv1beta3.Funcdef) string {
	if version, ok := fndef.Labels[rfv1beta3.LabelLambdaVersion]; ok {
		return version
	}
	return GetHash(fndef)
}

// XenvLabels infers a set of labels for corresponding deployment
func XenvLabels(xenv *rfv1beta3.Xenv) map[string]string {
	return map[string]string{
		rfv1beta3.LabelResType:  "xenv-pool",
		rfv1beta3.LabelExecutor: xenv.Name,
	}
}

// The number of times we retry updating a Trigger's status.
const statusUpdateRetries = 1

// UpdateFuncinstStatus ensures update to given status only if status changed
func UpdateFuncinstStatus(c rfcliv1.FuncinstInterface, funcinst *rfv1beta3.Funcinst, status rfv1beta3.FuncinstStatus) (*rfv1beta3.Funcinst, error) {
	oldStatus := funcinst.Status
	msg := func(t *rfv1beta3.Funcinst, i int) string {
		return fmt.Sprintf(
			"%s(%s,%d) status, active %d - %d",
			t.Name, t.ResourceVersion, i,
			// oldStatus.Phase, status.Phase,
			oldStatus.Active,
			status.Active,
		)
	}

	var getErr, updateErr error
	var updatedFni *rfv1beta3.Funcinst
	for i, t := 0, funcinst; ; i++ {
		// check if we need submit to apiserver
		// if t.Status.Phase == status.Phase && t.Status.Active == status.Active {
		// 	klog.V(4).Infof("no changes %s", msg(t, i))
		// 	return t, nil
		// }
		if t.Status.Active == status.Active && reflect.DeepEqual(t.Status.Conditions, status.Conditions) {
			klog.V(4).Infof("no changes %s", msg(t, i))
			return t, nil
		}

		t.Status = status
		updatedFni, updateErr = c.Update(context.TODO(), t, metav1.UpdateOptions{})
		if updateErr == nil {
			klog.V(3).Infof("updated %s", msg(updatedFni, i))
			return updatedFni, nil
		}
		// Stop retrying if we exceed statusUpdateRetries - the Refunc will be requeued with a rate limit.
		if i >= statusUpdateRetries {
			break
		}
		// Update the Refunc with the latest resource version for the next poll
		if t, getErr = c.Get(context.TODO(), t.Name, metav1.GetOptions{}); getErr != nil {
			// If the GET fails we can't trust status anymore. This error
			// is bound to be more interesting than the update failure.
			klog.V(3).Infof("failed updating %s, %v", msg(t, i), getErr)
			return nil, getErr
		}
	}

	klog.V(3).Infof("failed updating %s, %v", msg(funcinst, statusUpdateRetries), getErr)
	return nil, updateErr
}

func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

// IsRuntimePodReady returns true if given k8s resource is an executor pod and it has been initialized
func IsRuntimePodReady(obj metav1.Object) bool {
	if a, ok := obj.GetLabels()[rfv1beta3.LabelExecutorIsReady]; ok && a == "true" {
		return true
	}
	return false
}

// ExecutorPodName returns a printable name of an executor pod
func ExecutorPodName(pod *apiv1.Pod) string {
	parts := strings.Split(pod.Name, "-")
	return fmt.Sprintf(`%s(%s)`, parts[len(parts)-1], pod.Labels[rfv1beta3.LabelName])
}

func getObject(obj interface{}) (metav1.Object, bool) {
	ts, ok := obj.(cache.DeletedFinalStateUnknown)
	if ok {
		obj = ts.Obj
	}

	o, err := meta.Accessor(obj)
	if err != nil {
		return nil, false
	}
	return o, true
}

func getMD5Hash(object interface{}) string {
	md5Ctx := md5.New()
	data, _ := json.Marshal(object)
	md5Ctx.Write(data)
	return hex.EncodeToString(md5Ctx.Sum(nil))
}
