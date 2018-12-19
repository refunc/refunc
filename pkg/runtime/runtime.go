package runtime

import (
	"errors"
	"reflect"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	listerv1beta1 "k8s.io/client-go/listers/extensions/v1beta1"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/utils/rfutil"
)

// A Interface interface
type Interface interface {
	Name() string

	// IsPodReady checks if the given pod is runnable
	IsPodReady(pod *corev1.Pod) bool

	// GetDeploymentTemplate returns a deployment of the runner
	GetDeploymentTemplate(tpl *rfv1beta3.Xenv) *v1beta1.Deployment

	// InitPod initialize given pod
	// Note: one should not assume that the workDir still persist after InitPod being called
	InitPod(pod *corev1.Pod, funcinst *rfv1beta3.Funcinst, refunc *rfv1beta3.Funcdef, tpl *rfv1beta3.Xenv, workDir string) error
}

// well known errors
var (
	ErrRuntimeAlreadyExist = errors.New("runtime: A runtime with the same name already registered")
)

var registry struct {
	sync.Mutex
	runtimes map[string]Interface // runtime instances
}

// Register adds a new versioned runner to registry
func Register(r Interface) error {
	registry.Lock()
	defer registry.Unlock()
	if _, ok := registry.runtimes[r.Name()]; !ok {
		registry.runtimes[r.Name()] = r
		return nil
	}
	return ErrRuntimeAlreadyExist
}

// ForXenv returns runtime object for given xenv
func ForXenv(xenv *rfv1beta3.Xenv) Interface {
	registry.Lock()
	defer registry.Unlock()
	rtType := xenv.Spec.Type
	if rtType == "" {
		rtType = "agent"
	}
	if r, ok := registry.runtimes[rtType]; ok {
		return r
	}
	return nil
}

// GetXenvPoolDeployment returns runner template deployment for given refunc
func GetXenvPoolDeployment(lister listerv1beta1.DeploymentLister, xenv *rfv1beta3.Xenv) (*v1beta1.Deployment, error) {
	if xenv == nil {
		return nil, errors.New("runtime: xenv is nil")
	}
	deps, err := lister.Deployments(xenv.Namespace).List(labels.Set(rfutil.XenvLabels(xenv)).AsSelectorPreValidated())

	if err != nil {
		return nil, err
	}
	if len(deps) > 0 {
		ownerRef := xenv.AsOwner()
		for i := range deps {
			if ctlRef := metav1.GetControllerOf(deps[i]); ctlRef != nil && reflect.DeepEqual(ctlRef, ownerRef) {
				return deps[i], nil
			}
		}
	}
	return nil, nil
}

func init() {
	registry.runtimes = make(map[string]Interface)
}
