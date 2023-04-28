package funcinst

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/runtime"
	"github.com/refunc/refunc/pkg/utils"
	"github.com/refunc/refunc/pkg/utils/k8sutil"
	"github.com/refunc/refunc/pkg/utils/rfutil"
)

var errXenvNotFound = errors.New("tc: xenv not found")

// sync mainly do the following:
//  1. instantiates replicaset according to funcdef and xenv
//  2. specialize pods
//  3. creates hpa
func (rc *Controller) sync(key string) error {
	startTime := time.Now()
	defer func() {
		if dur := time.Since(startTime); dur > 90*time.Millisecond {
			klog.V(3).Infof("(tc) slow syncing %q (%v)", key, dur)
		}
	}()

	defer func() {
		if re := recover(); re != nil {
			utils.LogTraceback(re, 4, klog.V(1))
		}
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	fni, err := rc.funcinstLister.Funcinsts(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		klog.V(4).Infof("(tc) %q has been deleted", key)
		return nil
	}
	if err != nil {
		return err
	}

	if fni.Status.IsInactiveCondition() {
		// terminated funcinst, skip
		return nil
	}
	fni = fni.DeepCopy()

	// resolve funcdef
	fndefRef := fni.Spec.FuncdefRef
	fndef, err := rc.funcdefLister.Funcdeves(fndefRef.Namespace).Get(fndefRef.Name)
	if err != nil {
		if k8sutil.IsResourceNotFoundError(err) {
			klog.Warningf("(tc) funcdef for %s not found", key)
			// set current status to inactive,
			// notify funcinsts to stop servering new events,
			// when all pendings events finish, the funcinst and its resources will be collected
			_, err = rc.markFuncinstInactive(fni, "FuncdefRemoved", fmt.Sprintf("Funcdef %q is removed", fndefRef.Name))
		}
		return err
	}

	// check version
	oldHash, oldSpecHash := fni.Labels[rfv1beta3.LabelHash], fni.Labels[rfv1beta3.LabelSpecHash]
	newHash, newSpecHash := rfutil.GetHash(fndef), rfutil.GetSpecHash(fndef)
	if oldHash != newHash || oldSpecHash != newSpecHash {
		klog.V(3).Infof("(tc) %s funcdef hash changed %s -> %s %s -> %s", key, oldHash, newHash, oldSpecHash, newSpecHash)
		_, err = rc.markFuncinstInactive(fni, "FuncdefHashChanged", fmt.Sprintf("Funcdef %q hash is changed", fndefRef.Name))
		return err
	}

	// resolve xenv
	xenv, err := rc.getXenv(fndef)
	if err != nil {
		if _, updateErr := rc.markFuncinstPending(fni, "XenvNotResolved", fmt.Sprintf("Xenv is not resolved: %v", err)); updateErr != nil {
			return updateErr
		}
		if apierrors.IsNotFound(err) {
			klog.Warningf("(tc) xenv %q for %s not found", fndef.Spec.Runtime.Name, key)
			// fni will be re enqueued when xenv is ready
			return nil
		}
		return err
	}

	// lising related pods
	var pods []*corev1.Pod
	// get pods from local cache, this may not be the latest version
	if pods, err = rc.listPodsForFuncinst(fni, false); err != nil {
		return err
	}

	// init pods
	if len(pods) == 0 {
		rs, err := rc.getRuntimeReplciaSet(fni)
		if err != nil {
			return err
		}

		// executor rs is not found, create & init one
		if rs == nil {
			var pod *corev1.Pod
			rs, pod, err = rc.prepareRuntimeReplicaSet(fni, fndef, xenv)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				if fni.Status.Active > 0 {
					// update fni's status, due to
					//	* rs was deleted
					//	* active pods is greater than 0
					//	* we try to create rs again but failed
					status := fni.Status.DeepCopy()
					status.Active = 0
					status.SetActiveCondition("ReplicasetNotReady", fmt.Sprintf("RS setup error: %v", err)).ActiveCondition().Status = corev1.ConditionFalse
					if _, updateErr := rc.updateStatus(fni, *status); updateErr != nil {
						return updateErr
					}
				}
				return err
			}
			if pod != nil {
				pods = []*corev1.Pod{pod}
			}
		}

		if len(pods) == 0 && rs != nil {
			// try once more
			time.Sleep(10 * time.Millisecond)
			if pods, err = rc.listPodsForFuncinst(fni, false); err != nil {
				return err
			}
		}

		if len(pods) == 0 {
			// maybe rs is a recently created, we should return nil,
			// since up comming events(POD Change/Create) will wakeup this inst
			return nil
		}
	}

	var (
		nActive  int
		uninited []*corev1.Pod
	)
	for _, pod := range pods {
		running, _ := k8sutil.PodRunningAndReady(*pod)
		if running {
			if !rfutil.IsRuntimePodReady(pod) {
				uninited = append(uninited, pod)
			} else {
				nActive++
			}
		}
	}

	// initialize uninited pods
	if len(uninited) > 0 {
		xruntime := runtime.ForXenv(xenv)
		if xruntime == nil {
			klog.Warningf("(tc) unsupported xruntime %q for %q", xenv.Spec.Type, key)
			// xenv is missing, mark current funcinst inacitve
			_, err = rc.markFuncinstPending(fni, "UnsupportXRT", fmt.Sprintf("XRT of %q is not supported", xenv.Spec.Type))
			return err
		}

		klog.V(4).Infof("(tc) #%d pods need to be initialized", len(uninited))
		for _, pod := range uninited {
			if err = rc.initRuntimePod(fni, fndef, xenv, xruntime, pod); err != nil {
				// log error, try next
				klog.Warningf("(tc) failed to init pod for %q, %v", key, err)
				continue
			}
			nActive++
			status := fni.Status.DeepCopy()
			status.Active = nActive
			if nActive == 1 {
				status.SetActiveCondition("PodInitialized", "At least one pod of funcinst was initialized").Touch()
			}
			fni, err = rc.updateStatus(fni, *status)
			if err != nil {
				return err
			}
		}
	}

	// no is ready
	if nActive == 0 {
		status := fni.Status.DeepCopy()
		status.Active = 0
		status.SetActiveCondition("PodNotInitialized", "No pod was initialized").ActiveCondition().Status = corev1.ConditionFalse
		fni, err = rc.updateStatus(fni, *status)
		if err != nil {
			return err
		}
		return fmt.Errorf("tc: no active pod found for %q", key)
	}

	// check replicas
	if fndef.Spec.MaxReplicas > 1 && fndef.Spec.MaxReplicas > fndef.Spec.MinReplicas {
		if rc.hpaV2Lister != nil {
			hpa, err := rc.hpaV2Lister.HorizontalPodAutoscalers(fni.Namespace).Get(fni.Name)
			if hpa == nil || k8sutil.IsResourceNotFoundError(err) {
				klog.Infof("(tc) creating horizontalPodAutoscaler for %q", fni.Name)
				rs, err := rc.getRuntimeReplciaSet(fni)
				if err != nil {
					return err
				}
				hpa = rc.horizontalPodAutoscalerV2(fni, fndef, rs.GetName())
				if err = retryOnceOnError(func() error {
					hpa, err = rc.kclient.AutoscalingV2().HorizontalPodAutoscalers(fni.Namespace).Create(context.TODO(), hpa, metav1.CreateOptions{})
					if apierrors.IsAlreadyExists(err) {
						hpa, err = rc.getHorizontalPodAutoscalerV2(fni)
					}
					return err
				}); err == nil {
					return nil
				}
			}
			if err != nil {
				return err
			}
			if hpa.Spec.MaxReplicas != fndef.Spec.MaxReplicas {
				klog.V(3).Infof("(tc) updating horizontalPodAutoscaler for %q, from %d -> %d", fni.Name, hpa.Spec.MaxReplicas, fndef.Spec.MaxReplicas)
				return retryOnceOnError(func() error {
					hpa.Spec.MaxReplicas = fndef.Spec.MaxReplicas
					hpa, err = rc.kclient.AutoscalingV2().HorizontalPodAutoscalers(fni.Namespace).Update(context.TODO(), hpa, metav1.UpdateOptions{})
					if err != nil {
						hpa, err = rc.getHorizontalPodAutoscalerV2(fni)
					}
					return err
				})
			}
		} else {
			hpa, err := rc.hpaV1Lister.HorizontalPodAutoscalers(fni.Namespace).Get(fni.Name)
			if hpa == nil || k8sutil.IsResourceNotFoundError(err) {
				klog.Infof("(tc) creating horizontalPodAutoscaler for %q", fni.Name)
				rs, err := rc.getRuntimeReplciaSet(fni)
				if err != nil {
					return err
				}
				hpa = rc.horizontalPodAutoscalerV1(fni, fndef, rs.GetName())
				if err = retryOnceOnError(func() error {
					hpa, err = rc.kclient.AutoscalingV1().HorizontalPodAutoscalers(fni.Namespace).Create(context.TODO(), hpa, metav1.CreateOptions{})
					if apierrors.IsAlreadyExists(err) {
						hpa, err = rc.getHorizontalPodAutoscalerV1(fni)
					}
					return err
				}); err == nil {
					return nil
				}
			}
			if err != nil {
				return err
			}
			if hpa.Spec.MaxReplicas != fndef.Spec.MaxReplicas {
				klog.V(3).Infof("(tc) updating horizontalPodAutoscaler for %q, from %d -> %d", fni.Name, hpa.Spec.MaxReplicas, fndef.Spec.MaxReplicas)
				return retryOnceOnError(func() error {
					hpa.Spec.MaxReplicas = fndef.Spec.MaxReplicas
					hpa, err = rc.kclient.AutoscalingV1().HorizontalPodAutoscalers(fni.Namespace).Update(context.TODO(), hpa, metav1.UpdateOptions{})
					if err != nil {
						hpa, err = rc.getHorizontalPodAutoscalerV1(fni)
					}
					return err
				})
			}
		}
	}

	return nil
}

func (rc *Controller) handleChange(obj interface{}) {
	funcinst, ok := obj.(*rfv1beta3.Funcinst)
	if !ok {
		// maybe cache.DeletedFinalStateUnknown
		return
	}
	key, ok := rc.keyFunc(funcinst)
	if !ok {
		return
	}
	rc.enqueue(key, "Funcinst Change")
}

func (rc *Controller) handleUpdate(oldObj, curObj interface{}) {
	old := oldObj.(*rfv1beta3.Funcinst)
	cur := curObj.(*rfv1beta3.Funcinst)

	// Periodic resync may resend the deployment without changes in-between.
	// Also breaks loops created by updating the resource ourselves.
	if old.GetResourceVersion() == cur.GetResourceVersion() {
		return
	}
	if !rfv1beta3.OnlyLastActivityChanged(old, cur) {
		rc.handleChange(cur)
	}
}

func (rc *Controller) markFuncinstPending(funcinst *rfv1beta3.Funcinst, reason, message string) (*rfv1beta3.Funcinst, error) {
	return rc.updateStatus(funcinst, *funcinst.Status.DeepCopy().SetPendingCondition(reason, message))
}

func (rc *Controller) markFuncinstInactive(funcinst *rfv1beta3.Funcinst, reason, message string) (*rfv1beta3.Funcinst, error) {
	return rc.updateStatus(funcinst, *funcinst.Status.DeepCopy().SetInactiveCondition(reason, message))
}

func (rc *Controller) updateStatus(funcinst *rfv1beta3.Funcinst, status rfv1beta3.FuncinstStatus) (*rfv1beta3.Funcinst, error) {
	return rfutil.UpdateFuncinstStatus(rc.rclient.RefuncV1beta3().Funcinsts(funcinst.Namespace), funcinst.DeepCopy(), status)
}

func (rc *Controller) getXenv(fndef *rfv1beta3.Funcdef) (*rfv1beta3.Xenv, error) {
	return rc.xenvLister.Xenvs(fndef.Namespace).Get(fndef.Spec.Runtime.Name)
}
