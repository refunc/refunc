package funcinsts

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nuid"
	"golang.org/x/time/rate"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/env"
	"github.com/refunc/refunc/pkg/utils/rfutil"
)

func (r *Operator) handleFuncinstAdd(o interface{}) {
	fni := o.(*rfv1beta3.Funcinst)
	if fni.Labels[rfv1beta3.LabelTriggerType] == Type {
		r.indexOf(fni)
	}
}

func (r *Operator) handleFuncinstDelete(o interface{}) {
	fni, ok := o.(*rfv1beta3.Funcinst)
	if !ok {
		// it's cache.DeletedFinalStateUnknown
		return
	}
	if fni.Labels[rfv1beta3.LabelTriggerType] == Type {
		r.instsCacheForTrigger(r.triggerKey(fni)).Delete(fni)
	}
}

func (r *Operator) handleFuncinstUpdate(oldObj, curObj interface{}) {
	old := oldObj.(*rfv1beta3.Funcinst)
	cur := curObj.(*rfv1beta3.Funcinst)

	// Periodic resync may resend the deployment without changes in-between.
	// Also breaks loops created by updating the resource ourselves.
	if old.ResourceVersion == cur.ResourceVersion {
		return
	}

	if rfv1beta3.OnlyLastActivityChanged(old, cur) {
		return
	}

	if cur.Labels[rfv1beta3.LabelTriggerType] != Type {
		klog.Warningf("(fnio) %q trigger type changed from %q to %q", cur.Name, old.Labels[rfv1beta3.LabelTriggerType], cur.Labels[rfv1beta3.LabelTriggerType])
		r.instsCacheForTrigger(r.triggerKey(old)).Delete(old)
		return
	}

	klog.V(3).Infof(
		"(fnio) %s(%v) backends %d - %d, inactive %v",
		old.Name, cur.ResourceVersion,
		old.Status.Active, cur.Status.Active, cur.Status.IsInactiveCondition(),
	)
	r.indexOf(cur)

	trigger, err := r.TriggerLister.Triggers(cur.Spec.TriggerRef.Namespace).Get(cur.Spec.TriggerRef.Name)
	if err != nil {
		klog.Errorf("(fnio) resolve ref trigger error %v", err)
		return
	}
	r.setProvisionedInstance(trigger)
}

func (r *Operator) setProvisionedInstance(trigger *rfv1beta3.Trigger) (*rfv1beta3.Funcinst, error) {
	fndef, err := r.ResolveFuncdef(trigger)
	if err != nil {
		klog.Errorf("(fnio) set provisioned instance can't resolve funcdef %v", err)
		return nil, err
	}
	if fndef.Spec.MinReplicas > 0 {
		fni, err := r.GetFuncInstance(trigger)
		if err != nil {
			klog.Errorf("(fnio) set provisioned instance error %v", err)
		}
		return fni, err
	}
	return nil, nil
}

// GetFuncInstance returns or creates a refunc instance from trigger
func (r *Operator) GetFuncInstance(trigger *rfv1beta3.Trigger) (*rfv1beta3.Funcinst, error) {
	if trigger.Spec.Type != Type {
		return nil, fmt.Errorf("funcinst: unsupported trigger type %q", trigger.Spec.Type)
	}

	ns, name := trigger.Namespace, trigger.Name
	key := ns + "/" + name

	if fni, ok := r.getInstForTrigger(key); ok {
		return fni, nil
	}

	cache := r.instsCacheForTrigger(key)
	if !cache.limiter.Allow() {
		klog.V(3).Infof("(fnio) %q creating funcinst hit rate limits, %v/s", key, newFuncinstPerSec)
		if err := cache.limiter.Wait(r.ctx); err != nil {
			return nil, err
		}
		// try again, get funcinst from cache
		if fni, ok := r.getInstForTrigger(key); ok {
			return fni, nil
		}
	}

	// create from given trigger
	fndef, err := r.ResolveFuncdef(trigger)
	if err != nil {
		return nil, err
	}
	tr, err := r.TriggerLister.Triggers(ns).Get(name)
	if err != nil {
		return nil, err
	}

	klog.V(3).Infof("(fnio) creating new inst for %s", key)
	labels := rfutil.FuncinstLabels(fndef)
	labels[rfv1beta3.LabelTrigger] = name
	labels[rfv1beta3.LabelTriggerType] = Type
	fni := &rfv1beta3.Funcinst{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			// generate a unique name
			Name:   strings.ToLower(name + "-" + nuid.New().Next()[22-5:]),
			Labels: labels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: rfv1beta3.APIVersion,
					Kind:       rfv1beta3.FuncdefKind,
					Name:       fndef.Name,
					UID:        fndef.UID,
				},
			},
		},
		Spec: rfv1beta3.FuncinstSpec{
			FuncdefRef: fndef.Ref(),
			TriggerRef: tr.Ref(),
		},
	}

	// preparse runtime config
	fni.Spec.Runtime.Permissions = rfv1beta3.NewDefaultPermissions(fni, env.GlobalScopeRoot)
	fni.Spec.Runtime.Credentials.AccessKey, fni.Spec.Runtime.Credentials.SecretKey, err = r.credsP.IssueKeyPair(fni)
	if err != nil {
		return nil, err
	}
	// issue tokens, TODO: stop issue token for funcinst
	token, err := r.credsP.IssueAccessToken(fni)
	if err != nil {
		return nil, err
	}
	fni.Spec.Runtime.Credentials.Token = token

	// try to create and update created fni
	err = retryOnceOnError(func() error {
		newFni, err := r.RefuncClient.RefuncV1beta3().Funcinsts(ns).Create(context.TODO(), fni, metav1.CreateOptions{})
		if err == nil {
			fni = newFni
			return nil
		}
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	})
	if err != nil {
		klog.Errorf("(fnio) failed to create inst for %s, %v", key, err)
		return nil, err
	}

	r.indexOf(fni)
	return fni, nil
}

// Tap funcinst keeps it live
func (r *Operator) Tap(key string) {
	r.tappings.Update(key)
}

func (r *Operator) triggerKey(funcinst *rfv1beta3.Funcinst) string {
	return fmt.Sprintf("%s/%s", funcinst.Spec.TriggerRef.Namespace, funcinst.Spec.TriggerRef.Name)
}

func (r *Operator) instsCacheForTrigger(key string) *instsCache {
	value, _ := r.fninsts4Trigger.LoadOrStore(key, newInstsCacheFactory())
	return value.(instCacheFactory)()
}

// get or create a new funcinst
func (r *Operator) indexOf(fni *rfv1beta3.Funcinst) {
	cache := r.instsCacheForTrigger(r.triggerKey(fni))
	if !fni.Status.IsInactiveCondition() {
		cache.Index(fni)
	} else {
		cache.Delete(fni)
	}
}

func (r *Operator) getInstForTrigger(key string) (fni *rfv1beta3.Funcinst, has bool) {
	if val, ok := r.fninsts4Trigger.Load(key); ok {
		cache := val.(instCacheFactory)()
		cache.fniIndexCache.Range(func(k interface{}, value interface{}) bool {
			idx := value.(*funcinstIndex)
			if inst, err := r.FuncinstLister.Funcinsts(idx.ns).Get(idx.name); err == nil && !inst.Status.IsInactiveCondition() {
				fni = inst
				has = true
				return false
			}
			// remove invalid from cache
			cache.fniIndexCache.Delete(k)
			return true
		})
		return
	}
	return nil, false
}

func (r *Operator) tappingFuncinsts(stopC <-chan struct{}) {
	ticker := time.NewTicker(r.TappingInterval)
	defer ticker.Stop()
	defer klog.Info("(fnio) tapping service stopped")

	activeFuncs := make(map[string]time.Time)
	tappingSink := r.tappings.Observe()

	for {
		select {
		case <-tappingSink.Changes():
			key := tappingSink.Next().(string)
			// active mark
			activeFuncs[key] = time.Now()
			continue
		case <-stopC:
			return
		case <-ticker.C:
		}
		if len(activeFuncs) == 0 {
			continue
		}

		// time's up for tapping
		for key := range activeFuncs {
			splitted := strings.SplitN(key, "/", 2)
			if len(splitted) != 2 {
				continue
			}
			ns, name := splitted[0], splitted[1]
			fni, err := r.FuncinstLister.Funcinsts(ns).Get(name)
			if err != nil {
				klog.Warningf("(tapping) failed get %q, %v", key, err)
				continue
			}
			if fni.Status.IsInactiveCondition() {
				// skip tapping funcinst that is inactive
				continue
			}
			if fni.Status.LastActivity().After(activeFuncs[key]) {
				// already tapped by other operator
				continue
			}

			// nolint:errcheck
			retryOnceOnError(func() error {
				fni.Status.ActiveCondition().LastUpdateTime = activeFuncs[key].Format(time.RFC3339)
				if fni, err = r.RefuncClient.RefuncV1beta3().Funcinsts(ns).Update(context.TODO(), fni, metav1.UpdateOptions{}); err == nil {
					return nil
				}
				// Update the Refunc with the latest resource version for the next poll
				if updated, err := r.RefuncClient.RefuncV1beta3().Funcinsts(ns).Get(context.TODO(), name, metav1.GetOptions{}); err != nil {
					// If the GET fails we can't trust status anymore. This error
					// is bound to be more interesting than the update failure.
					klog.V(3).Infof("(tapping) failed updating %q, %v", key, err)
				} else {
					fni = updated
				}
				return err
			})
		}

		klog.V(4).Infof("(tapping) tapped #%d refuncs", len(activeFuncs))
		activeFuncs = make(map[string]time.Time)
	}
}

type funcinstIndex struct {
	ns, name string
}

type instsCache struct {
	limiter       *rate.Limiter
	fniIndexCache sync.Map
}

func (tc *instsCache) Index(fni *rfv1beta3.Funcinst) {
	tc.fniIndexCache.Store(k8sKey(fni), &funcinstIndex{fni.Namespace, fni.Name})
}

func (tc *instsCache) Delete(fni *rfv1beta3.Funcinst) {
	tc.fniIndexCache.Delete(k8sKey(fni))
}

type instCacheFactory func() *instsCache

const newFuncinstPerSec rate.Limit = 3

func newInstsCacheFactory() instCacheFactory {
	// local states, make it can lazily initialize
	var (
		once  sync.Once
		cache *instsCache
	)

	return func() *instsCache {
		once.Do(func() {
			cache = &instsCache{
				limiter: rate.NewLimiter(newFuncinstPerSec, 1),
			}
		})
		return cache
	}
}
