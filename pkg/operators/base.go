package operators

import (
	"strings"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	refunc "github.com/refunc/refunc/pkg/generated/clientset/versioned"
	rfinformers "github.com/refunc/refunc/pkg/generated/informers/externalversions"
	rflistersv1 "github.com/refunc/refunc/pkg/generated/listers/refunc/v1beta3"
)

// well known default constants
const (
	MaxRetries = 2
)

// BaseOperator syncs and manages funcinsts.
type BaseOperator struct {
	// configured
	Namespace string

	// shared by concrete funcinst
	RefuncClient    refunc.Interface
	RefuncInformers rfinformers.SharedInformerFactory
	FuncdefLister   rflistersv1.FuncdefLister
	TriggerLister   rflistersv1.TriggerLister
	WantedInformers []cache.InformerSynced
}

// NewBaseOperator creates a new refunc router from config
func NewBaseOperator(
	cfg *rest.Config,
	rclient refunc.Interface,
	refuncInformers rfinformers.SharedInformerFactory,
) (router *BaseOperator, err error) {

	r := new(BaseOperator)

	r.RefuncClient = rclient
	r.RefuncInformers = refuncInformers

	// config listers
	r.FuncdefLister = refuncInformers.Refunc().V1beta3().Funcdeves().Lister()
	r.TriggerLister = refuncInformers.Refunc().V1beta3().Triggers().Lister()

	r.WantedInformers = []cache.InformerSynced{
		r.RefuncInformers.Refunc().V1beta3().Funcdeves().Informer().HasSynced,
		r.RefuncInformers.Refunc().V1beta3().Triggers().Informer().HasSynced,
	}

	return r, nil
}

// GetNamespace implements interface, returns namespace current managed on
func (r *BaseOperator) GetNamespace() string {
	return r.Namespace
}

// WaitForCacheSync will not return until stopC is closed.
func (r *BaseOperator) WaitForCacheSync(stopC <-chan struct{}) bool {
	klog.Info("(o) waiting for listers to be fully synced")
	return cache.WaitForCacheSync(stopC, r.WantedInformers...)
}

// ResolveFuncdef returns funcdef specfied by trigger
func (r *BaseOperator) ResolveFuncdef(trigger *rfv1beta3.Trigger) (*rfv1beta3.Funcdef, error) {
	ids := strings.SplitN(trigger.Spec.FuncName, "/", 2)
	var ns, name string
	if len(ids) == 1 {
		ns, name = trigger.Namespace, ids[0]
	} else {
		ns, name = ids[0], ids[1]
	}
	return r.FuncdefLister.Funcdeves(ns).Get(name)
}
