package funcinst

import (
	"context"

	autoscalev2beta1 "k8s.io/api/autoscaling/v2beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/utils/rfutil"
)

func (rc *Controller) getHorizontalPodAutoscaler(funcinst *rfv1beta3.Funcinst) (*autoscalev2beta1.HorizontalPodAutoscaler, error) {
	as, err := rc.kclient.AutoscalingV2beta1().HorizontalPodAutoscalers(funcinst.Namespace).Get(context.TODO(), funcinst.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return as, nil
}

var (
	minReplicas  int32 = 1
	targetCPU    int32 = 90
	targetMemory int32 = 200
)

func (rc *Controller) horizontalPodAutoscaler(funcinst *rfv1beta3.Funcinst, fndef *rfv1beta3.Funcdef, rsName string) *autoscalev2beta1.HorizontalPodAutoscaler {
	as := &autoscalev2beta1.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      funcinst.Name,
			Namespace: funcinst.Namespace,
			Labels:    rfutil.ExecutorLabels(funcinst),
		},
		Spec: autoscalev2beta1.HorizontalPodAutoscalerSpec{
			MinReplicas: &minReplicas,
			MaxReplicas: fndef.Spec.MaxReplicas,
			Metrics: []autoscalev2beta1.MetricSpec{
				autoscalev2beta1.MetricSpec{
					Type: autoscalev2beta1.ResourceMetricSourceType,
					Resource: &autoscalev2beta1.ResourceMetricSource{
						Name:                     "cpu",
						TargetAverageUtilization: &targetCPU,
					},
				},
				autoscalev2beta1.MetricSpec{
					Type: autoscalev2beta1.ResourceMetricSourceType,
					Resource: &autoscalev2beta1.ResourceMetricSource{
						Name:                     "memory",
						TargetAverageUtilization: &targetMemory,
					},
				},
			},
			ScaleTargetRef: autoscalev2beta1.CrossVersionObjectReference{
				Kind:       "ReplicaSet",
				Name:       rsName,
				APIVersion: "apps/v1",
			},
		},
	}
	// set owner
	ownerRef := funcinst.AsOwner()
	ownerRef.Controller = &isController
	as.OwnerReferences = append(as.OwnerReferences, *ownerRef)
	return as
}
