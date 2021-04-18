package util

import (
	corev1 "k8s.io/api/core/v1"
	mpaTypes "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"
	"multidim-pod-autoscaler/pkg/util/patch"
)

// PatchCalculator 计算指定POD的patch
type PatchCalculator interface {
	CalculatePatches(pod *corev1.Pod, mpa *mpaTypes.MultidimPodAutoscaler) ([]patch.Patch, error)
}
