package patch

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	admissionUtil "multidim-pod-autoscaler/pkg/admission/util"
	v1 "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"
	containerUtil "multidim-pod-autoscaler/pkg/util/container"
	patchUtil "multidim-pod-autoscaler/pkg/util/patch"
	"multidim-pod-autoscaler/pkg/util/recommendation"
	"strings"
)

const (
	// ResourceUpdatesAnnotation 为资源被 MPA 修改的标记名
	ResourceUpdatesAnnotation = "MpaUpdates"
)

type resourceUpdatesPatchCalculator struct {
	recommendationProvider admissionUtil.RecommendationProvider
}

// NewResourceUpdatesPatchCalculator 创建一个新的 resourceUpdatesPatchCalculator
func NewResourceUpdatesPatchCalculator(recommendationProvider admissionUtil.RecommendationProvider) admissionUtil.PatchCalculator {
	return &resourceUpdatesPatchCalculator{
		recommendationProvider: recommendationProvider,
	}
}

// CalculatePatches 计算给定pod下的容器的资源修改patches
// 并打上被mpa修改的标记
func (r *resourceUpdatesPatchCalculator) CalculatePatches(
	pod *corev1.Pod,
	mpa *v1.MultidimPodAutoscaler,
) ([]patchUtil.Patch, error) {
	patches := make([]patchUtil.Patch, 0)

	containersRecommendedResources, annotationsPerContainer, err := r.recommendationProvider.GetContainerResourcesForPod(pod, mpa)
	if err != nil {
		return patches, fmt.Errorf("failed to calculate resource patch for pod %v/%v: %v", pod.Namespace, pod.Name, err)
	}

	if annotationsPerContainer == nil {
		annotationsPerContainer = recommendation.ContainerAnnotationsMap{}
	}

	updatesAnnotations := make([]string, 0)
	for i, containerRecomm := range containersRecommendedResources {
		newPatches, newAnnotations, newUpdateAnnotation := getContainerPatch(pod, i, containerRecomm)
		patches = append(patches, newPatches...)
		annotationsPerContainer[pod.Spec.Containers[i].Name] = append(annotationsPerContainer[pod.Spec.Containers[i].Name], newAnnotations...)
		updatesAnnotations = append(updatesAnnotations, newUpdateAnnotation)
	}

	if len(updatesAnnotations) > 0 {
		mpaAnnotationValue := fmt.Sprintf("Pod resources updated by %s: %s", mpa.Name, strings.Join(updatesAnnotations, "; "))
		patches = append(patches, GetAddAnnotationsPatch(ResourceUpdatesAnnotation, mpaAnnotationValue))
	}
	return patches, nil
}

// getContainerPatch 获取指定容器的 patches
func getContainerPatch(
	pod *corev1.Pod, containerIndex int,
	containerResources containerUtil.Resources,
) ([]patchUtil.Patch, []string, string) {

	patches := make([]patchUtil.Patch, 0)
	annotations := make([]string, 0)

	// 如果为空，添加空 Resource 对象
	if pod.Spec.Containers[containerIndex].Resources.Limits == nil &&
		pod.Spec.Containers[containerIndex].Resources.Requests == nil {
		patches = append(patches, getEmptyResourcesPatch(containerIndex))
	}

	// 获取 requests 字段的patches
	requestPatches, requestAnnotations := getResourcesPatch(
		containerIndex, "requests",
		pod.Spec.Containers[containerIndex].Resources.Requests, containerResources.Requests,
	)
	// 获取 limits 字段的patches
	limitPatches, limitAnnotations := getResourcesPatch(
		containerIndex, "limits",
		pod.Spec.Containers[containerIndex].Resources.Limits, containerResources.Limits,
	)
	patches = append(patches, requestPatches...)
	patches = append(patches, limitPatches...)

	annotations = append(annotations, requestAnnotations...)
	annotations = append(annotations, limitAnnotations...)

	updateContainerAnnotation := fmt.Sprintf("container %d: ", containerIndex) + strings.Join(annotations, ", ")

	return patches, annotations, updateContainerAnnotation
}

// getResourcesPatch 获取指定pod资源字段的资源配置patch
// subFieldName 可取 ["limits" | "requests"]
func getResourcesPatch(
	containerIndex int, subFieldName string,
	postResource corev1.ResourceList, newResource corev1.ResourceList,
) ([]patchUtil.Patch, []string) {
	patches := make([]patchUtil.Patch, 0)
	annotations := make([]string, 0)
	if postResource == nil && len(newResource) > 0 {
		// 在该container的spec中，没有指定 subFieldaName 的资源限制
		// 而需要将 newResource 加入进去，需要先添加一个新的字段
		patches = append(patches, getEmptyResourcesSubFieldPatch(containerIndex, subFieldName))
	}
	// 将 newResource 中指定了的资源加入到pod的spec中
	for resourceName, quantity := range newResource {
		patches = append(patches, getResourcesSubFieldPatch(containerIndex, subFieldName, resourceName, quantity))
		annotations = append(annotations, fmt.Sprintf("%s-%s", resourceName, subFieldName))
	}
	return patches, annotations
}

// getResourcesSubFieldPatch 获取如下的 patch
// .spec.containers[i].resources.{
//  	[Limits|Requests]: {
// 			resourceName: ...
// 		}
// }
func getResourcesSubFieldPatch(
	containerIndex int, subFieldName string,
	resourceName corev1.ResourceName, quantity resource.Quantity,
) patchUtil.Patch {
	return patchUtil.Patch{
		Op:    patchUtil.Add,
		Path:  fmt.Sprintf("/spec/containers/%d/resources/%s/%s", containerIndex, subFieldName, resourceName),
		Value: quantity.String(),
	}
}

// getEmptyResourcesPatch 获取如下 patch
// .spec.containers[i].resources.{
//  	Limits: {}
// 		Requests: {}
// }
func getEmptyResourcesPatch(containerIndex int) patchUtil.Patch {
	return patchUtil.Patch{
		Op:    patchUtil.Add,
		Path:  fmt.Sprintf("/spec/containers/%d/resources", containerIndex),
		Value: corev1.ResourceRequirements{},
	}
}

// getEmptyResourcesSubFieldPatch 获取如下 patch
// .spec.containers[i].resources.{
//  	[Limits|Requests]: {}
// }
// subFieldName 为 "Limits" 或 "Requests"
func getEmptyResourcesSubFieldPatch(containerIndex int, subFieldName string) patchUtil.Patch {
	return patchUtil.Patch{
		Op:    patchUtil.Add,
		Path:  fmt.Sprintf("/spec/containers/%d/resources/%s", containerIndex, subFieldName),
		Value: corev1.ResourceList{},
	}
}
