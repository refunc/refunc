package xenv

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
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

func (rc *Controller) sync(key string) error {
	startTime := time.Now()
	defer func() {
		if dur := time.Since(startTime); dur > 30*time.Millisecond {
			klog.V(3).Infof("(tc) finished syncing %q (%v)", key, dur)
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

	xenv, err := rc.xenvLister.Xenvs(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		klog.V(3).Infof("(xc) %q has been deleted", key)
		return nil
	}
	if err != nil {
		return err
	}

	if !rc.hasRef(xenv) {
		return nil
	}

	rt := runtime.ForXenv(xenv)
	if rt == nil {
		return fmt.Errorf("xc: unknown runtime %s for xenv %s", xenv.Spec.Type, key)
	}

	dep, err := runtime.GetXenvPoolDeployment(rc.deploymentLister, xenv)

	if xenv.Spec.PoolSize > 0 {
		if k8sutil.IsResourceNotFoundError(err) || (err == nil && dep == nil) {
			dep := rc.getDeployment(rt, xenv)
			_, err := rc.kclient.AppsV1().Deployments(xenv.Namespace).Create(context.TODO(), dep, metav1.CreateOptions{})
			switch {
			case apierrors.IsAlreadyExists(err):
				klog.Infof("(xc) %s pool already created", xenv.Name)
				return nil
			case err != nil:
				return err
			}
			klog.Infof("(xc) %s pool deployed", key)
			return nil
		}
		if err != nil {
			return err
		}
		// upgrade pool if needed
		tgt := rc.getDeployment(rt, xenv)
		if dep.Spec.Template.Spec.InitContainers[0].Image != tgt.Spec.Template.Spec.InitContainers[0].Image {
			// we need recreate pool
			return rc.kclient.AppsV1().Deployments(dep.Namespace).Delete(context.TODO(), dep.Name, *k8sutil.CascadeDeleteOptions(0))
		}
		return k8sutil.PatchDeployment(rc.kclient, xenv.Namespace, dep.Name, func(d *appsv1.Deployment) {
			for k, v := range tgt.Labels {
				d.Labels[k] = v
			}
			d.Spec.Replicas = tgt.Spec.Replicas
			d.Spec.Template.Spec.Containers = tgt.Spec.Template.Spec.Containers
			d.Spec.Template.Spec.Volumes = tgt.Spec.Template.Spec.Volumes
		})
	}

	if dep != nil {
		klog.V(3).Infof("(xc) %s scale pool size to 0", key)
		return rc.kclient.AppsV1().Deployments(dep.Namespace).Delete(context.TODO(), dep.Name, *k8sutil.CascadeDeleteOptions(0))
	}

	return nil
}

var isController = true

func (rc *Controller) getDeployment(r runtime.Interface, xenv *rfv1beta3.Xenv) *appsv1.Deployment {
	dep := r.GetDeploymentTemplate(xenv)
	// override meta,
	// using name starts with 0 ensure relabled pods always be the first element in backends
	dep.Name = "xpool-" + xenv.Name
	dep.Namespace = xenv.Namespace
	if dep.Labels == nil {
		dep.Labels = make(map[string]string)
	}
	for k, v := range rfutil.XenvLabels(xenv) {
		dep.Labels[k] = v
	}
	dep.Spec.Selector.MatchLabels = dep.Labels
	dep.Spec.Template.Labels = dep.Labels
	// set owner
	ownerRef := xenv.AsOwner()
	ownerRef.Controller = &isController
	dep.OwnerReferences = append(dep.OwnerReferences, *ownerRef)
	return dep
}

func (rc *Controller) handleChange(obj interface{}) {
	xenv, ok := obj.(*rfv1beta3.Xenv)
	if !ok {
		// maybe cache.DeletedFinalStateUnknown
		return
	}
	key, ok := rc.keyFunc(xenv)
	if !ok {
		return
	}
	rc.enqueue(key, "Xenv Change")
}
