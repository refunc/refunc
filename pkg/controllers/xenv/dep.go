package xenv

import (
	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/utils/k8sutil"
	"github.com/refunc/refunc/pkg/utils/rfutil"
	"k8s.io/klog"
)

func (rc *Controller) handleDeploymentChange(o interface{}) {
	if obj, ok := rfutil.IsXenvRes(o); ok {
		xenvName, ok := obj.GetLabels()[rfv1beta3.LabelExecutor]
		if !ok {
			klog.Warningf("(xc) got xenv pool %s/%s without xenv's name", obj.GetNamespace(), obj.GetName())
			return
		}
		xenv, err := rc.xenvLister.Xenvs(obj.GetNamespace()).Get(xenvName)
		if err != nil {
			if !k8sutil.IsResourceNotFoundError(err) && obj.GetDeletionTimestamp() == nil {
				klog.Errorf("(xc) cannot resolve xenv %s/%s, %v", obj.GetNamespace(), obj.GetName(), err)
			}
			return
		}
		rc.enqueue(xenv, "Deployment Change")
	}
}
