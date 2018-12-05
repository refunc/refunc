package funcinst

import (
	"fmt"
	"reflect"

	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/runtime"
	"github.com/refunc/refunc/pkg/utils/rfutil"
)

func (rc *Controller) getRuntimeReplciaSet(funcinst *rfv1beta3.Funcinst) (*v1beta1.ReplicaSet, error) {
	rss, err := rc.kclient.ExtensionsV1beta1().ReplicaSets(funcinst.Namespace).List(metav1.ListOptions{
		LabelSelector: labels.Set(rfutil.ExecutorLabels(funcinst)).String(),
	})

	if err != nil {
		return nil, err
	}
	if len(rss.Items) > 0 {
		ownerRef := funcinst.AsOwner()
		for i := range rss.Items {
			if rss.Items[i].DeletionTimestamp != nil {
				continue
			}
			if ctlRef := metav1.GetControllerOf(&rss.Items[i]); ctlRef != nil && reflect.DeepEqual(ctlRef, ownerRef) {
				return &rss.Items[i], nil
			}
		}
	}
	return nil, nil
}

func (rc *Controller) prepareRuntimeReplicaSet(funcinst *rfv1beta3.Funcinst, fndef *rfv1beta3.Funcdef, xenv *rfv1beta3.Xenv) (rs *v1beta1.ReplicaSet, pod *v1.Pod, err error) {

	var dep *v1beta1.Deployment
	if xenv.Spec.PoolSize > 0 {
		// relabel a pod if xenv has a pool
		dep, err = runtime.GetXenvPoolDeployment(rc.deploymentLister, xenv)
		if dep != nil {
			// relabel a pod from pool, make it a live server
			pod, err = rc.relabelPodFromDeployment(funcinst, dep)
			if err != nil {
				klog.Warningf("(tc) failed to relabel pod in pool for xenv %s/%s, %v", xenv.Namespace, xenv.Name, err)
			}
		} else if err != nil {
			klog.Warningf("(tc) failed to get dep for xenv %s/%s, %v", xenv.Namespace, xenv.Name, err)
		}
		// reset error
		err = nil
	}

	defer func() {
		if err != nil && pod != nil {
			klog.Errorf("rc: failed to gen rs for %q, %v, deleting relabeled pod %v",
				funcinst.Name,
				err,
				rc.kclient.Core().Pods(funcinst.Namespace).Delete(pod.GetName(), new(metav1.DeleteOptions)),
			)
		}
	}()

	// ensure dep is not nil
	if dep == nil {
		rt := runtime.ForXenv(xenv)
		if rt == nil {
			err = fmt.Errorf("(tc) failed to get runtime for %s/%s", xenv.Namespace, xenv.Name)
			return
		}
		dep = rt.GetDeploymentTemplate(xenv)
	}

	// creating a replicas from template
	rs = rc.replicaSetFromTemplate(funcinst, dep)
	err = retryOnceOnError(func() error {
		rs, err = rc.kclient.Extensions().ReplicaSets(funcinst.Namespace).Create(rs)
		if apierrors.IsAlreadyExists(err) {
			err = nil
		}
		return err
	})
	if err != nil {
		return
	}

	klog.V(2).Infof("(tc) created rs %q for %q, hot pod %v", rs.Name, funcinst.Name, pod != nil)
	return nil, pod, nil
}

var (
	initReplicas int32 = 1
	isController       = true
)

func (rc *Controller) replicaSetFromTemplate(funcinst *rfv1beta3.Funcinst, dep *v1beta1.Deployment) *v1beta1.ReplicaSet {

	labels := rfutil.ExecutorLabels(funcinst)
	rs := &v1beta1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: funcinst.Name + "-",
			Namespace:    funcinst.Namespace,
			Labels:       labels,
		},
		Spec: v1beta1.ReplicaSetSpec{
			Replicas: &initReplicas,
			Template: dep.Spec.Template,
		},
	}
	// override pointers
	rs.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: labels,
	}
	rs.Spec.Template.Labels = labels
	// set owner
	ownerRef := funcinst.AsOwner()
	ownerRef.Controller = &isController
	rs.OwnerReferences = append(rs.OwnerReferences, *ownerRef)

	return rs
}
