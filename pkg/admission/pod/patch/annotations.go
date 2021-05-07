package patch

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"multidim-pod-autoscaler/pkg/admission/util"
	v1 "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"
	"multidim-pod-autoscaler/pkg/util/annotations"
	"multidim-pod-autoscaler/pkg/util/patch"
)

// observedPodPatchCalculator implements the Calculator interface to calculate Annotations patch
type observedPodPatchCalculator struct{}

// CalculatePatches implements the Calculator interface to calculate Annotations patch
func (*observedPodPatchCalculator) CalculatePatches(pod *corev1.Pod, mpa *v1.MultidimPodAutoscaler) ([]patch.Patch, error) {
	mpaObservedPodValue := annotations.GetMpaObservedPodAnnotationsValue(pod)
	return []patch.Patch{
		GetAddAnnotationsPatch(annotations.MpaObservedPodAnnotations, mpaObservedPodValue),
	}, nil
}

// NewObservedPodPatchCalculator 返回一个新的observedPodPatchCalculator
func NewObservedPodPatchCalculator() util.PatchCalculator {
	return &observedPodPatchCalculator{}
}

// GetEmptyAddAnnotationsPatch 返回一个添加空annotations的patch
func GetEmptyAddAnnotationsPatch() patch.Patch {
	return patch.Patch{
		Op:    patch.Add,
		Path:  "/metadata/annotations",
		Value: map[string]string{},
	}
}

// GetAddAnnotationsPatch 返回一个添加 .metadata.annotations/... 的 patch
func GetAddAnnotationsPatch(annotationsName, annotationsValue string) patch.Patch {
	return patch.Patch{
		Op:    patch.Add,
		Path:  fmt.Sprintf("/metadata/annotations/%s", annotationsName),
		Value: annotationsValue,
	}
}
