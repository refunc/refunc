package xenv

import (
	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
)

func (rc *Controller) handleFuncdefChange(obj interface{}) {
	fndef, ok := obj.(*rfv1beta3.Funcdef)
	if !ok {
		return
	}
	if fndef.Spec.Runtime.Name == "" {
		return
	}

	xenv, err := rc.xenvLister.Xenvs(fndef.Namespace).Get(fndef.Spec.Runtime.Name)
	if err == nil && xenv != nil {
		rc.enqueue(xenv, "Funcdef Change")
	}
}

func (rc *Controller) handleFuncdefUpdate(o, c interface{}) {
	old, cur := o.(*rfv1beta3.Funcdef), c.(*rfv1beta3.Funcdef)
	if old.ResourceVersion == cur.ResourceVersion {
		return
	}
	if old.Spec.Runtime.Name != cur.Spec.Runtime.Name {
		// need wake old xenv
		rc.handleFuncdefChange(old)
	}
	rc.handleFuncdefChange(cur)
}

func (rc *Controller) hasRef(xenv *rfv1beta3.Xenv) bool {
	fndefs, err := rc.funcdefLister.Funcdeves(xenv.Namespace).List(labels.Everything())
	if err != nil {
		klog.Errorf("(xc) failed to list funcdef, %v", err)
		return false
	}
	for _, fndef := range fndefs {
		if fndef.Spec.Runtime.Name == xenv.Name {
			return true
		}
	}
	return false
}
