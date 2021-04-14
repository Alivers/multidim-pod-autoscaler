package annotations

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"strings"
)

const (
	// MpaObservedPodAnnotations 作为被MPA监控的POD的.metadata.annotations的name
	MpaObservedPodAnnotations = "mpaObservedPod"
	// stringSeparator 用作容器名之间的分隔符
	// 将容器名join起来用作.metadata.annotations的value
	stringSeparator = ", "
)

// GetMpaObservedPodAnnotationsValue 返回指定Pod的.metadata.annotations的value
// 使用pod下容器名的组合作为该value的结果
func GetMpaObservedPodAnnotationsValue(pod *corev1.Pod) string {
	containersName := make([]string, len(pod.Spec.Containers))

	for i := range pod.Spec.Containers {
		containersName[i] = pod.Spec.Containers[i].Name
	}

	return strings.Join(containersName, stringSeparator)
}

// ParseMpaObservedPodAnnotationsValue 解析给定annotationsValue为容器名的列表
func ParseMpaObservedPodAnnotationsValue(annotaionsValue string) ([]string, error) {
	if annotaionsValue == "" {
		return []string{}, nil
	}
	containersName := strings.Split(annotaionsValue, stringSeparator)
	for i := range containersName {
		if errs := validation.IsDNS1123Label(containersName[i]); len(errs) != 0 {
			return nil, fmt.Errorf("incorrect format: %s is not a valid container name: %v", containersName[i], errs)
		}
	}

	return containersName, nil
}
