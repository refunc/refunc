package xenv

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/runtime"
	"github.com/refunc/refunc/pkg/utils/k8sutil"
)

// gcMonitor collecting garbage of runners
func (rc *Controller) gcMonitor(stopC <-chan struct{}) {
	klog.Infof("(xc) refunc gc started at %v", rc.GCInterval)
	t0 := time.Now()
	defer func() { klog.Infof("(xc) refunc gc stopped, using %v", time.Since(t0)) }()

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
	klog.V(4).Info("(xc:gc) start collecting garbage")
	t0 := time.Now()
	defer func() { klog.V(4).Infof("(xc:gc) garbage collected using %v", time.Since(t0)) }()

	xenvset := make(map[string]struct{})
	setval := struct{}{}
	err := cache.ListAll(
		rc.refuncInformers.Refunc().V1beta3().Funcdeves().Informer().GetIndexer(),
		labels.Everything(),
		func(m interface{}) {
			fndef, ok := m.(*rfv1beta3.Funcdef)
			if !ok {
				return
			}
			if fndef.Spec.Runtime.Name == "" {
				return
			}
			xenvset[fndef.Namespace+"/"+fndef.Spec.Runtime.Name] = setval
		},
	)
	if err != nil {
		klog.Warningf("(xc:gc) failed to collect funcdefs, %v", err)
		return
	}

	err = cache.ListAll(
		rc.refuncInformers.Refunc().V1beta3().Xenvs().Informer().GetIndexer(),
		labels.Everything(),
		func(m interface{}) {
			xenv, ok := m.(*rfv1beta3.Xenv)
			if !ok {
				// it's cache.DeletedFinalStateUnknown
				return
			}
			key := xenv.Namespace + "/" + xenv.Name
			if _, hasRef := xenvset[key]; !hasRef {
				if err := rc.cleanup(xenv); err != nil {
					klog.Warningf("(xc:gc) failed to cleanup xenv %s, %v", key, err)
				}
			}
		},
	)
	if err != nil {
		klog.Warningf("(xc:gc) failed to collect xenvs, %v", err)
		return
	}

	// collect orphans
	if err := rc.collectOrphanDeployments(); err != nil {
		klog.Warningf("(tc:gc) failed to collect orphan deployments, %v", err)
		return
	}
}

func (rc *Controller) cleanup(xenv *rfv1beta3.Xenv) error {
	dep, err := runtime.GetXenvPoolDeployment(rc.deploymentLister, xenv)
	if err != nil && !k8sutil.IsResourceNotFoundError(err) {
		klog.Warningf("(xc:gc) cannot find deployment for %s/%s, %v", xenv.Namespace, xenv.Name, err)
	}
	if dep != nil {
		return rc.kclient.AppsV1().Deployments(dep.Namespace).Delete(context.TODO(), dep.Name, *k8sutil.CascadeDeleteOptions(0))
	}

	return nil
}

func (rc *Controller) collectOrphanDeployments() error {
	return cache.ListAll(
		rc.kubeInformers.Apps().V1().Deployments().Informer().GetIndexer(),
		labels.Everything(),
		func(m interface{}) {
			rs, ok := m.(*appsv1.Deployment)
			if !ok {
				// it's cache.DeletedFinalStateUnknown
				return
			}

			if ctlRef := metav1.GetControllerOf(rs); ctlRef != nil {
				if ctlRef.Kind != rfv1beta3.XenvKind || ctlRef.APIVersion != rfv1beta3.APIVersion {
					return
				}
				if _, err := rc.xenvLister.Xenvs(rs.Namespace).Get(ctlRef.Name); k8sutil.IsResourceNotFoundError(err) && rs.DeletionTimestamp == nil {
					klog.V(3).Infof("(xc:gc) cleanup orphan dep %s/%s", rs.Namespace, rs.Name)
					err = retryOnceOnError(func() error {
						err = rc.kclient.AppsV1().Deployments(rs.Namespace).Delete(context.TODO(), rs.Name, *k8sutil.CascadeDeleteOptions(0))
						if k8sutil.IsResourceNotFoundError(err) {
							return nil
						}
						return err
					})
					if err != nil {
						klog.Errorf("(xc:gc) delete orphan dep %s/%s failed, %v", rs.Namespace, rs.Name, err)
					}

				}
			}
		},
	)
}

func retryOnceOnError(fn func() error) error {
	for i := 0; ; i++ {
		err := fn()
		if err != nil {
			if i >= 1 {
				return err
			}
			continue
		}
		return nil
	}
}
