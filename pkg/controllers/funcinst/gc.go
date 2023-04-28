package funcinst

import (
	"context"
	"fmt"
	"math"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/utils/k8sutil"
)

// gcMonitor collecting garbage of runners
func (rc *Controller) gcMonitor(stopC <-chan struct{}) {
	klog.Infof("(tc) refunc gc started at %v", rc.GCInterval)
	t0 := time.Now()
	defer func() { klog.Infof("(tc) refunc gc stopped, using %v", time.Since(t0)) }()

	ticker := time.NewTicker(rc.GCInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
		case <-stopC:
			return
		}

		rc.collectGarbadge()
	}
}

func (rc *Controller) collectGarbadge() {
	klog.V(4).Info("(tc:gc) start collecting garbage")
	t0 := time.Now()
	defer func() { klog.V(4).Infof("(tc:gc) garbage collected using %v", time.Since(t0)) }()

	fniCounter := make(map[string]int32)
	err := cache.ListAll(
		rc.refuncInformers.Refunc().V1beta3().Funcinsts().Informer().GetIndexer(),
		labels.Everything(),
		func(m interface{}) {
			funcinst, ok := m.(*rfv1beta3.Funcinst)
			if !ok {
				// it's cache.DeletedFinalStateUnknown
				return
			}

			key := funcinst.Namespace + "/" + funcinst.Name
			fndefRef := funcinst.Spec.FuncdefRef
			fnkey := fndefRef.Namespace + "/" + fndefRef.Name
			fndef, err := rc.funcdefLister.Funcdeves(fndefRef.Namespace).Get(fndefRef.Name)
			if err != nil {
				klog.Errorf("(tc:gc) %q missing funcdef", key)
			}
			if fndef == nil {
				return
			}
			timeoutDur := time.Duration(fndef.Spec.Runtime.Timeout) * time.Second
			if timeoutDur == 0 {
				timeoutDur = 60 * time.Second
			}

			if err == nil && !funcinst.Status.IsInactiveCondition() {
				// check if we should skip checking
				if func() (skip bool) {
					cnt := fniCounter[fnkey]
					if cnt == -1 {
						klog.Errorf("(tc:gc) %q(%s) max replicas reached", fnkey, funcinst.Name)
						// step into inactive
						rc.markFuncinstInactive(funcinst, "MaxReplicasReached", fmt.Sprintf("Max replicas %d reached", fndef.Spec.MaxReplicas))
						// collected next turn
						return false
					}

					if fndef.Spec.MinReplicas > 0 && cnt < fndef.Spec.MinReplicas {
						fniCounter[fnkey] = cnt + int32(funcinst.Status.Active)
						return true
					}

					if time.Since(funcinst.Status.LastActivity()) < time.Duration(math.Max(float64(rc.IdleDuraion), float64(timeoutDur))) {
						if cnt+int32(funcinst.Status.Active) >= fndef.Spec.MaxReplicas {
							// max replicas reached, the following funinst with same funcdef will be marked as inactive
							fniCounter[fnkey] = -1
						} else {
							fniCounter[fnkey] = cnt + int32(funcinst.Status.Active)
						}
						return true
					}
					klog.Infof("(tc:gc) %q is old enough to be collected", key)
					return false
				}() {
					// we should skip this turn
					return
				}

				rc.markFuncinstInactive(funcinst, "Collected", "Collected by GC")
			}

			deleteNow := func() bool {
				for _, cond := range funcinst.Status.Conditions {
					if cond.Type == rfv1beta3.FuncinstInactive {
						return cond.Reason == "FuncdefHashChanged" || cond.Reason == "FuncdefRemoved" || cond.Reason == "UnsupportXRT"
					}
				}
				return false
			}()

			if !deleteNow && fndef != nil && time.Since(funcinst.Status.LastActivity()) < timeoutDur {
				// delete until idle long enough
				klog.Infof("(tc:gc) %q will be removed in %v", key, time.Until(funcinst.Status.LastActivity().Add(timeoutDur)))
				return
			}

			err = rc.rclient.RefuncV1beta3().Funcinsts(funcinst.Namespace).Delete(context.TODO(), funcinst.Name, *k8sutil.CascadeDeleteOptions(0))
			if err != nil && !k8sutil.IsResourceNotFoundError(err) {
				klog.Errorf("(tc:gc) fail to delete %q, %v", key, err)
				return
			}

			klog.Infof("(tc:gc) collected %q", key)
		},
	)

	if err != nil {
		klog.Warningf("(tc:gc) failed to collect funcinsts, %v", err)
	}

}
