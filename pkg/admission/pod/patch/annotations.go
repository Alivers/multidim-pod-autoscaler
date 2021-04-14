package patch

import (
	"fmt"
	"multidim-pod-autoscaler/pkg/util/patch"
)

// GetEmptyAddAnnotationsPatch 返回一个添加空annotations的patch
func GetEmptyAddAnnotationsPatch() patch.Patch {
	return patch.Patch{
		Op:    patch.Add,
		Path:  "metadata/annotations",
		Value: map[string]string{},
	}
}

// GetAddAnnotationsPatch 返回一个添加 .metadata.annotations/... 的 patch
func GetAddAnnotationsPatch(annotationsName, annotationsValue string) patch.Patch {
	return patch.Patch{
		Op:    patch.Add,
		Path:  fmt.Sprintf("metadata/annotations/%s", annotationsName),
		Value: annotationsValue,
	}
}
