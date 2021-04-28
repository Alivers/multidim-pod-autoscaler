package recommendation

import (
	corev1 "k8s.io/api/core/v1"
	mpaTypes "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"
	"multidim-pod-autoscaler/pkg/recommender/metrics"
)

type Calculator interface {
	Calculate(mpa *mpaTypes.MultidimPodAutoscaler, controlledPod []*corev1.Pod) *mpaTypes.RecommendedResources
}

type calculator struct {
	metricsClient metrics.Client
}

func NewCalculator(client metrics.Client) Calculator {
	return &calculator{
		metricsClient: client,
	}
}

func (c *calculator) Calculate(
	mpa *mpaTypes.MultidimPodAutoscaler,
	controlledPod []*corev1.Pod,
) *mpaTypes.RecommendedResources {
	panic("implement me")
}
