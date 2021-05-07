package util

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"
	mpaTypes "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"
	containerUtil "multidim-pod-autoscaler/pkg/util/container"
	"multidim-pod-autoscaler/pkg/util/limitrange"
	recommendationUtil "multidim-pod-autoscaler/pkg/util/recommendation"
)

// RecommendationProvider 获取指定 pod 的容器资源
type RecommendationProvider interface {
	GetContainerResourcesForPod(
		pod *corev1.Pod,
		mpa *mpaTypes.MultidimPodAutoscaler,
	) ([]containerUtil.Resources, recommendationUtil.ContainerAnnotationsMap, error)
}

type recommendationProvider struct {
	limitRange              limitrange.Calculator
	recommendationProcessor recommendationUtil.Processor
}

// NewRecommendationProvider 返回一个新的 RecommendationProvider
func NewRecommendationProvider(
	calculator limitrange.Calculator,
	processor recommendationUtil.Processor) RecommendationProvider {
	return &recommendationProvider{
		limitRange:              calculator,
		recommendationProcessor: processor,
	}
}

// GetContainerResourcesForPod 获取指定pod的容器的推荐资源(包含limit request的形式)
// admission需要将该信息写入pod.spec来创建新的pod
func (r *recommendationProvider) GetContainerResourcesForPod(
	pod *corev1.Pod,
	mpa *mpaTypes.MultidimPodAutoscaler,
) ([]containerUtil.Resources, recommendationUtil.ContainerAnnotationsMap, error) {
	if mpa == nil || pod == nil {
		klog.V(2).Infof("connot get recommendations, MPA(%v) or Pod(%v) is nil", mpa, pod)
		return nil, nil, nil
	}
	var containerLimitRange *corev1.LimitRangeItem
	var err error
	if r.limitRange != nil {
		containerLimitRange, err = r.limitRange.GetContainerLimitRangeItem(pod.Namespace)
		if err != nil {
			return nil, nil, fmt.Errorf("error getting container LimitRange: %s", err)
		}
	}

	var resourcePolicy *mpaTypes.PodResourcePolicy
	if mpa.Spec.UpdatePolicy == nil || mpa.Spec.UpdatePolicy.UpdateMode == nil || *mpa.Spec.UpdatePolicy.UpdateMode != mpaTypes.UpdateModeOff {
		resourcePolicy = mpa.Spec.ResourcePolicy
	}
	// 获取 limit request 形式的资源(伸缩之后)
	containerResources, annotations := getContainersResources(pod, resourcePolicy, mpa.Status.RecommendationResources, containerLimitRange)

	return containerResources, annotations, nil
}

// getContainersResources 获取容器的推荐资源
func getContainersResources(
	pod *corev1.Pod,
	podPolicy *mpaTypes.PodResourcePolicy,
	recommendation *mpaTypes.RecommendedResources,
	limitRange *corev1.LimitRangeItem,
) ([]containerUtil.Resources, recommendationUtil.ContainerAnnotationsMap) {
	if recommendation == nil {
		return nil, nil
	}

	resources := make([]containerUtil.Resources, len(pod.Spec.Containers))

	annotations := make(recommendationUtil.ContainerAnnotationsMap)

	for i, container := range pod.Spec.Containers {
		// 获取容器的推荐资源
		containerRecomm := recommendationUtil.GetContainerRecommendation(container.Name, recommendation.ContainerRecommendations)

		if containerRecomm == nil {
			klog.V(2).Infof("no matching recommendation found for container %s", container.Name)
			continue
		} else {
			// 推荐不为空 设为requests
			resources[i].Requests = containerRecomm.Target
		}

		var defaultLimit corev1.ResourceList
		if limitRange != nil {
			defaultLimit = limitRange.Default
		}
		//containerControlledMode := mpaApi.GetContainerControlledMode(container.Name, podPolicy)
		//if containerControlledMode == mpaTypes.ContainerControlledRequestsAndLimits {
		// 需要同时伸缩 request 和 limit
		recommLimit, anotation := containerUtil.GetProportionalLimit(
			container.Resources.Limits, container.Resources.Requests,
			resources[i].Requests, defaultLimit)

		if recommLimit != nil {
			// 设置伸缩后的limit
			resources[i].Limits = recommLimit
			if len(anotation) > 0 {
				annotations[container.Name] = anotation
			}
		} else {
			resources[i].Limits = resources[i].Requests
			annotations[container.Name] = []string{"EmptydefaultLimits, set to the same with Requests"}
		}
		//}
		klog.V(4).Infof("container(%s) of pod(%s/%s)'s recommendation: request-%v limits-%v", container.Name, pod.Namespace, pod.Name, resources[i].Requests, resources[i].Limits)
	}
	return resources, annotations
}
