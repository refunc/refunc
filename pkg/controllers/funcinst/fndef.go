package funcinst

import (
	"reflect"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

func (rc *Controller) handleFuncdefChange(obj interface{}) {
	fndef, ok := obj.(*rfv1beta3.Funcdef)
	if !ok {
		return
	}
	ref := fndef.Ref()

	var l = 0
	cache.ListAll(
		rc.refuncInformers.Refunc().V1beta3().Funcinsts().Informer().GetIndexer(),
		labels.Everything(),
		func(m interface{}) {
			fni, ok := m.(*rfv1beta3.Funcinst)
			if !ok {
				// it's cache.DeletedFinalStateUnknown
				return
			}
			if reflect.DeepEqual(ref, fni.Spec.FuncdefRef) {
				rc.enqueue(fni, "Funcdef Change")
				l++
			}
		},
	)
	if l > 0 {
		klog.V(2).Infof("(tc) affected %d funcinsts", l)
	}
}
