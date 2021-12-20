package funcinsts

import (
	"context"
	"fmt"
	"sync"
	"time"

	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	observer "github.com/refunc/go-observer"
	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/builtins"
	"github.com/refunc/refunc/pkg/credsyncer"
	refunc "github.com/refunc/refunc/pkg/generated/clientset/versioned"
	informers "github.com/refunc/refunc/pkg/generated/informers/externalversions"
	rflistersv1 "github.com/refunc/refunc/pkg/generated/listers/refunc/v1beta3"
	operators "github.com/refunc/refunc/pkg/operators"
	"github.com/refunc/refunc/pkg/transport"
	"github.com/refunc/refunc/pkg/utils"
	"github.com/refunc/refunc/pkg/utils/k8sutil"
)

// Operator creates http based rpc trigger automatically for every funcs
type Operator struct {
	*operators.BaseOperator

	// configured
	TappingInterval time.Duration

	// shared by concrete funcinst
	FuncinstLister rflistersv1.FuncinstLister

	// trasnport handler
	handler transport.OperatorHandler

	credsP credsyncer.Provider

	// internal
	endpoint2Trigger sync.Map
	fninsts4Trigger  sync.Map

	ctx    context.Context
	cancel context.CancelFunc

	// tapping when servering hosts
	tappings observer.Property
}

const (
	// Type name for rpc trigger
	Type = "eventgateway"

	labelAutoCreated = "funcinsts.refunc.io/auto-created"
)

// NewOperator creates a new rpc trigger operator
func NewOperator(
	cfg *rest.Config,
	rclient refunc.Interface,
	rfInformers informers.SharedInformerFactory,
	handler transport.OperatorHandler,
	creds credsyncer.Provider,
) (*Operator, error) {
	base, err := operators.NewBaseOperator(cfg, rclient, rfInformers)
	if err != nil {
		return nil, err
	}

	r := &Operator{
		BaseOperator:   base,
		FuncinstLister: rfInformers.Refunc().V1beta3().Funcinsts().Lister(),
		handler:        handler,
		credsP:         creds,
		tappings:       observer.NewProperty(nil),
	}
	r.WantedInformers = append(r.WantedInformers, r.RefuncInformers.Refunc().V1beta3().Funcinsts().Informer().HasSynced)

	return r, nil
}

// Run will not return until stopC is closed.
func (r *Operator) Run(stopC <-chan struct{}) {
	defer func() {
		if re := recover(); re != nil {
			utils.LogTraceback(re, 4, klog.V(1))
		}
	}()

	r.ctx, r.cancel = context.WithCancel(context.Background())

	// add events emitter
	r.RefuncInformers.Refunc().V1beta3().Funcdeves().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.handleFuncdefAdd,
		DeleteFunc: r.handleFuncdefDelete,
	})
	r.RefuncInformers.Refunc().V1beta3().Triggers().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.handleTriggerAdd,
		DeleteFunc: r.handleTriggerDelete,
	})
	r.RefuncInformers.Refunc().V1beta3().Funcinsts().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.handleFuncinstAdd,
		UpdateFunc: r.handleFuncinstUpdate,
		DeleteFunc: r.handleFuncinstDelete,
	})

	// start funcinst operator
	if !r.BaseOperator.WaitForCacheSync(stopC) {
		klog.Error("(funcinsts) cannot fully sync resources")
		return
	}

	klog.Info("(fnio) staring tapping service")
	go r.tappingFuncinsts(stopC)

	klog.Infof("(funcinsts) setup and listen on endpoints, with builtins: %v", builtins.ListBuiltins())

	go r.handler.Start(r.ctx, r)

	<-stopC
	klog.Info("(funcinsts) shuting down events operator")
}

func (r *Operator) handleFuncdefAdd(o interface{}) {
	fndef := o.(*rfv1beta3.Funcdef)

	trs, err := r.getTriggerLister()(labels.Everything())
	if err != nil {
		klog.Errorf("(funcinsts) failed to list triggers, %v", err)
	}
	for _, trigger := range trs {
		if trigger.Spec.Type == Type && trigger.Spec.FuncName == fndef.Name {
			return
		}
	}
	klog.V(3).Infof("(funcinsts) auto generate trigger for %s", k8sKey(fndef))
	// not found we should create
	trigger := &rfv1beta3.Trigger{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: fndef.Namespace,
			Name:      fndef.Name,
			Labels: map[string]string{
				labelAutoCreated: "true",
			},
			Annotations: map[string]string{
				rfv1beta3.AnnotationRPCVer: "v2",
			},
		},
		Spec: rfv1beta3.TriggerSpec{
			Type:     Type,
			FuncName: fndef.Name,
		},
	}
	err = retryOnceOnError(func() error {
		_, err := r.RefuncClient.RefuncV1beta3().Triggers(fndef.Namespace).Create(context.TODO(), trigger, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	})
	if err != nil {
		klog.Errorf("(funcinsts) failed to generate trigger for %s, %v", k8sKey(fndef), err)
	}
}

func (r *Operator) handleFuncdefDelete(o interface{}) {
	fndef, ok := o.(*rfv1beta3.Funcdef)
	if !ok {
		// it's cache.DeletedFinalStateUnknown
		return
	}

	trs, err := r.getTriggerLister()(labels.Everything())
	if err != nil {
		klog.Errorf("(funcinsts) failed to list triggers, %v", err)
	}

	// tirggers may point to same func with different aliases
	var triggers []*rfv1beta3.Trigger
	for _, tr := range trs {
		if tr.Spec.Type == Type && tr.Spec.FuncName == fndef.Name {
			triggers = append(triggers, tr)
		}
	}
	if len(triggers) == 0 {
		return
	}

	// check if it is created by system
	for _, trigger := range triggers {
		if val, ok := trigger.Labels[labelAutoCreated]; ok && val == "true" {
			klog.V(3).Infof("(funcinsts) delete auto generated trigger %s", k8sKey(trigger))
			err := retryOnceOnError(func() error {
				err := r.RefuncClient.RefuncV1beta3().Triggers(trigger.Namespace).Delete(context.TODO(), trigger.Name, *k8sutil.CascadeDeleteOptions(0))
				if apierrors.IsNotFound(err) {
					return nil
				}
				return err
			})
			if err != nil {
				klog.Errorf("(funcinsts) failed to delete trigger %s for %s, %v", k8sKey(trigger), k8sKey(fndef), err)
			}
		}
	}
}

type triggerIndex struct {
	ns, name string
}

func (r *Operator) handleTriggerAdd(o interface{}) {
	trigger := o.(*rfv1beta3.Trigger)
	if trigger.Spec.Type != Type {
		// skip other triggers
		return
	}

	r.endpoint2Trigger.Store(r.endpointForTrigger(trigger), &triggerIndex{trigger.Namespace, trigger.Name})
}

func (r *Operator) handleTriggerDelete(o interface{}) {
	trigger, ok := o.(*rfv1beta3.Trigger)
	if !ok {
		// it's cache.DeletedFinalStateUnknown
		return
	}
	if trigger.Spec.Type != Type {
		// skip other triggers
		return
	}

	key := r.endpointForTrigger(trigger)
	if _, ok := r.endpoint2Trigger.Load(key); ok {
		klog.V(3).Infof("(funcinsts) delete trigger %s", key)
		r.endpoint2Trigger.Delete(key)
	}
}

func (r *Operator) endpointForTrigger(trigger *rfv1beta3.Trigger) string {
	name := trigger.Spec.FuncName
	if trigger.Spec.Event != nil && trigger.Spec.Event.Alias != "" {
		name = trigger.Spec.Event.Alias
	}
	return trigger.Namespace + "/" + name
}

func (r *Operator) getTriggerLister() (trlsiter func(labels.Selector) ([]*rfv1beta3.Trigger, error)) {
	if r.Namespace == apiv1.NamespaceAll {
		trlsiter = r.TriggerLister.List
		return
	}
	trlsiter = r.TriggerLister.Triggers(r.Namespace).List
	return
}

// TriggerForEndpoint returns trigger specified by endpoint
func (r *Operator) TriggerForEndpoint(endpoint string) (*rfv1beta3.Trigger, error) {
	val, ok := r.endpoint2Trigger.Load(endpoint)
	if !ok {
		return nil, fmt.Errorf("%q not found", endpoint)
	}
	idx := val.(*triggerIndex)
	return r.TriggerLister.Triggers(idx.ns).Get(idx.name)
}

func k8sKey(o metav1.Object) string {
	return o.GetNamespace() + "/" + o.GetName()
}

func retryOnceOnError(fn func() error) error {
	for i := 0; ; i++ {
		err := fn()
		if err != nil {
			if i >= operators.MaxRetries {
				return err
			}
			continue
		}
		return nil
	}
}
