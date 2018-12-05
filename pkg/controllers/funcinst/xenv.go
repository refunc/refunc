package funcinst

import (
	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
)

func (rc *Controller) handleXenvChange(o interface{}) {
	xenv, ok := o.(*rfv1beta3.Xenv)
	if !ok {
		// it's cache.DeletedFinalStateUnknown
		return
	}

	funcinsts, err := rc.funcinstLister.Funcinsts(xenv.Namespace).List(labels.Everything())
	if err != nil {
		klog.Errorf("(tc) failed to list funcinsts, %v", err)
		return
	}

	// wake up all related funcinsts
	for _, funcinst := range funcinsts {
		fndef, err := rc.funcdefLister.Funcdeves(xenv.Namespace).Get(funcinst.Spec.FuncdefRef.Name)
		if err != nil {
			klog.Errorf("(tc) cannot resolve funcdef of %s/%s for xenv %s/%s, %v", funcinst.Namespace, funcinst.Name, xenv.Namespace, xenv.Name, err)
			continue
		}
		if fndef.Spec.Runtime != nil && fndef.Spec.Runtime.Name == xenv.Name {
			rc.enqueue(funcinst, "Xenv Change")
		}
	}
}
