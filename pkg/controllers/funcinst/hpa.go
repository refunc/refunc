package funcinst

import (
	"context"

	autoscalev1 "k8s.io/api/autoscaling/v1"
	autoscalev2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/utils/rfutil"
)

func (rc *Controller) getHorizontalPodAutoscalerV2(funcinst *rfv1beta3.Funcinst) (*autoscalev2.HorizontalPodAutoscaler, error) {
	as, err := rc.kclient.AutoscalingV2().HorizontalPodAutoscalers(funcinst.Namespace).Get(context.TODO(), funcinst.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return as, nil
}

func (rc *Controller) getHorizontalPodAutoscalerV1(funcinst *rfv1beta3.Funcinst) (*autoscalev1.HorizontalPodAutoscaler, error) {
	as, err := rc.kclient.AutoscalingV1().HorizontalPodAutoscalers(funcinst.Namespace).Get(context.TODO(), funcinst.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return as, nil
}

var (
	minReplicas  int32 = 1
	targetCPU    int32 = 90
	targetMemory int32 = 90
)

func (rc *Controller) horizontalPodAutoscalerV2(funcinst *rfv1beta3.Funcinst, fndef *rfv1beta3.Funcdef, rsName string) *autoscalev2.HorizontalPodAutoscaler {
	as := &autoscalev2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      funcinst.Name,
			Namespace: funcinst.Namespace,
			Labels:    rfutil.ExecutorLabels(funcinst),
		},
		Spec: autoscalev2.HorizontalPodAutoscalerSpec{
			MinReplicas: &minReplicas,
			MaxReplicas: fndef.Spec.MaxReplicas,
			Metrics: []autoscalev2.MetricSpec{
				{
					Type: autoscalev2.ResourceMetricSourceType,
					Resource: &autoscalev2.ResourceMetricSource{
						Name: "cpu",
						Target: autoscalev2.MetricTarget{
							Type:               autoscalev2.UtilizationMetricType,
							AverageUtilization: &targetCPU,
						},
					},
				},
				{
					Type: autoscalev2.ResourceMetricSourceType,
					Resource: &autoscalev2.ResourceMetricSource{
						Name: "memory",
						Target: autoscalev2.MetricTarget{
							Type:               autoscalev2.UtilizationMetricType,
							AverageUtilization: &targetMemory,
						},
					},
				},
			},
			ScaleTargetRef: autoscalev2.CrossVersionObjectReference{
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

func (rc *Controller) horizontalPodAutoscalerV1(funcinst *rfv1beta3.Funcinst, fndef *rfv1beta3.Funcdef, rsName string) *autoscalev1.HorizontalPodAutoscaler {
	as := &autoscalev1.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      funcinst.Name,
			Namespace: funcinst.Namespace,
			Labels:    rfutil.ExecutorLabels(funcinst),
		},
		Spec: autoscalev1.HorizontalPodAutoscalerSpec{
			MinReplicas:                    &minReplicas,
			MaxReplicas:                    fndef.Spec.MaxReplicas,
			TargetCPUUtilizationPercentage: &targetCPU,
			ScaleTargetRef: autoscalev1.CrossVersionObjectReference{
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
