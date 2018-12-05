package funcinst

import (
	"fmt"
	"math"
	"time"

	autoscalev1 "k8s.io/api/autoscaling/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
					if time.Since(funcinst.Status.LastActivity()) < time.Duration(math.Max(float64(rc.IdleDuraion), float64(timeoutDur))) {
						if cnt+1 >= fndef.Spec.MaxReplicas {
							// max replicas reached, the following funinst with same funcdef will be marked as inactive
							fniCounter[fnkey] = -1
						} else {
							fniCounter[fnkey] = cnt + 1
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
				klog.Infof(
					"(tc:gc) %q will be removed in %v",
					key,
					funcinst.Status.LastActivity().Add(timeoutDur).Sub(time.Now()),
				)
				return
			}

			err = rc.rclient.RefuncV1beta3().Funcinsts(funcinst.Namespace).Delete(funcinst.Name, k8sutil.CascadeDeleteOptions(0))
			if err != nil && !k8sutil.IsResourceNotFoundError(err) {
				klog.Errorf("(tc:gc) fail to delete %q, %v", key, err)
				return
			}

			if err := rc.cleanup(funcinst); err != nil {
				klog.Errorf("(tc:gc) fail to cleanup %q, %v", key, err)
				return
			}
			klog.Infof("(tc:gc) collected %q", key)
		},
	)
	if err != nil {
		klog.Warningf("(tc:gc) failed to collect funcinsts, %v", err)
	}

	// collect orphans
	if err := rc.collectOrphanReplicas(); err != nil {
		klog.Warningf("(tc:gc) failed to collect orphan replicas, %v", err)
	}
	if err := rc.collectOrphanHPAs(); err != nil {
		klog.Warningf("(tc:gc) failed to collect orphan HAPs, %v", err)
	}
}

func (rc *Controller) cleanup(funcinst *rfv1beta3.Funcinst) error {
	rs, err := rc.getRuntimeReplciaSet(funcinst)
	if err != nil && !k8sutil.IsResourceNotFoundError(err) {
		klog.Warningf("(tc:gc) cannot find replicas for %s/%s, %v", funcinst.Namespace, funcinst.Name, err)
	}
	if rs != nil {
		return rc.kclient.Extensions().ReplicaSets(rs.Namespace).Delete(rs.Name, k8sutil.CascadeDeleteOptions(0))
	}

	hpa, err := rc.getHorizontalPodAutoscaler(funcinst)
	if err != nil && !k8sutil.IsResourceNotFoundError(err) {
		klog.Warningf("(tc:gc) cannot find HPA for %s/%s, %v", funcinst.Namespace, funcinst.Name, err)
	}
	if hpa != nil {
		return rc.kclient.AutoscalingV1().HorizontalPodAutoscalers(hpa.Namespace).Delete(hpa.Name, k8sutil.CascadeDeleteOptions(0))
	}

	return nil
}

func (rc *Controller) collectOrphanReplicas() error {
	return cache.ListAll(
		rc.kubeInformers.Extensions().V1beta1().ReplicaSets().Informer().GetIndexer(),
		labels.Everything(),
		func(m interface{}) {
			rs, ok := m.(*v1beta1.ReplicaSet)
			if !ok {
				// it's cache.DeletedFinalStateUnknown
				return
			}
			if ctlRef := metav1.GetControllerOf(rs); ctlRef != nil {
				if ctlRef.Kind != rfv1beta3.FuncinstKind || ctlRef.APIVersion != rfv1beta3.APIVersion {
					return
				}
				if fni, err := rc.funcinstLister.Funcinsts(rs.Namespace).Get(ctlRef.Name); (fni != nil && fni.Spec.FuncdefRef == nil) || (k8sutil.IsResourceNotFoundError(err) && rs.DeletionTimestamp == nil) {
					klog.V(3).Infof("(tc:gc) cleanup orphan rs %s/%s", rs.Namespace, rs.Name)
					err = retryOnceOnError(func() error {
						err = rc.kclient.Extensions().ReplicaSets(rs.Namespace).Delete(rs.Name, k8sutil.CascadeDeleteOptions(0))
						if k8sutil.IsResourceNotFoundError(err) {
							return nil
						}
						return err
					})
					if err != nil {
						klog.Errorf("(tc:gc) delete orphan rs %s/%s failed, %v", rs.Namespace, rs.Name, err)
					}

				}
			}
		},
	)
}

func (rc *Controller) collectOrphanHPAs() error {
	return cache.ListAll(
		rc.kubeInformers.Autoscaling().V1().HorizontalPodAutoscalers().Informer().GetIndexer(),
		labels.Everything(),
		func(m interface{}) {
			as, ok := m.(*autoscalev1.HorizontalPodAutoscaler)
			if !ok {
				// it's cache.DeletedFinalStateUnknown
				return
			}
			if ctlRef := metav1.GetControllerOf(as); ctlRef != nil {
				if ctlRef.Kind != rfv1beta3.FuncinstKind || ctlRef.APIVersion != rfv1beta3.APIVersion {
					return
				}
				if fni, err := rc.funcinstLister.Funcinsts(as.Namespace).Get(ctlRef.Name); (fni != nil && fni.Spec.FuncdefRef == nil) || (k8sutil.IsResourceNotFoundError(err) && as.DeletionTimestamp == nil) {
					klog.V(3).Infof("(tc:gc) cleanup orphan HPA %s/%s", as.Namespace, as.Name)
					err = retryOnceOnError(func() error {
						err = rc.kclient.AutoscalingV1().HorizontalPodAutoscalers(as.Namespace).Delete(as.Name, k8sutil.CascadeDeleteOptions(0))
						if k8sutil.IsResourceNotFoundError(err) {
							return nil
						}
						return err
					})
					if err != nil {
						klog.Errorf("(tc:gc) delete orphan HPA %s/%s failed, %v", as.Namespace, as.Name, err)
					}
				}
			}
		},
	)
}
