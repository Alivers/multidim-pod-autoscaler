package recommendation

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog"
	mpaTypes "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"
	"multidim-pod-autoscaler/pkg/util/limitrange"
	mpaApi "multidim-pod-autoscaler/pkg/util/mpa"
)

// ContainerAnnotationsMap 为 容器名 到 容器的 annotations 的映射
type ContainerAnnotationsMap map[string][]string

// Processor 处理 recommender 推荐的资源方案，使其符合约束控制等
type Processor interface {
	// AdjustRecommendation 调整推荐的资源方案
	// 考虑因素：容器资源限制(limitrange); pod资源策略(用户预配置)等
	// 调整后的推荐方案 和 error 必须有一个不为空
	// 返回的ContainerAnnotationsMap包含了被处理容器和其对应的处理标记的map
	AdjustRecommendation(
		podRecommendation *mpaTypes.RecommendedResources,
		policy *mpaTypes.PodResourcePolicy,
		pod *corev1.Pod,
	) (*mpaTypes.RecommendedResources, ContainerAnnotationsMap, error)
}

// 推荐资源方案的调整动作; 用作调整标记 annotations
type adjustAction string

const (
	adjustToMinAllowed adjustAction = "adjust to min allowed"
	adjustToMaxAllowed adjustAction = "adjust to max allowed"
	adjustToLimit      adjustAction = "adjust to container limit"
	adjustToMaxLimit   adjustAction = "adjust to fix Max limit in limit range"
	adjustToMinLimit   adjustAction = "adjust to fix Min limit in limit range"
)

type processor struct {
	limitRangeCalculator limitrange.Calculator
}

func NewProcessor(limitrangeCalculator limitrange.Calculator) Processor {
	return &processor{
		limitRangeCalculator: limitrangeCalculator,
	}
}

// AdjustRecommendation 调整推荐的资源方案
// 考虑因素：容器资源限制(limitrange); pod资源策略(用户预配置)等
// 调整后的推荐方案 和 error 必须有一个不为空
// 返回的ContainerAnnotationsMap包含了被处理容器和其对应的处理标记的map
func (p *processor) AdjustRecommendation(
	podRecommendation *mpaTypes.RecommendedResources,
	policy *mpaTypes.PodResourcePolicy,
	pod *corev1.Pod,
) (*mpaTypes.RecommendedResources, ContainerAnnotationsMap, error) {
	if podRecommendation == nil || policy == nil {
		return nil, nil, nil
	}

	var adjustedRecommendations []mpaTypes.RecommendedContainerResources
	containersAnnotations := ContainerAnnotationsMap{}

	limitAdjustedRecomendations, err := p.adjustToPodLimitRange(podRecommendation.ContainerRecommendations, pod)
	if err != nil {
		return nil, nil, err
	}

	for _, containerRecomm := range limitAdjustedRecomendations {
		// 获取容器对象
		container := getContainer(containerRecomm.ContainerName, pod)

		if container == nil {
			klog.V(2).Infof("no matching container(name: %s) found for adjust recommendation", containerRecomm.ContainerName)
			continue
		}
		// 获取容器的 limit range
		containerLimitRange, err := p.limitRangeCalculator.GetContainerLimitRangeItem(pod.Namespace)
		if err != nil {
			klog.Warningf("failed to fetch limit range for %v namespace", pod.Namespace)
		}
		// 调整推荐方案
		adjustedContainerResource, containerAnnotations, err := adjustRecommendationForContainer(*container, &containerRecomm, policy, containerLimitRange)
		// 添加该容器的处理标记
		if len(containerAnnotations) > 0 {
			containersAnnotations[container.Name] = containerAnnotations
		}
		if err != nil {
			return nil, nil, fmt.Errorf("connot update recommendation for Container %s", container.Name)
		}
		// 保存当前容器调整后的推荐方案
		adjustedRecommendations = append(adjustedRecommendations, *adjustedContainerResource)
	}
	return &mpaTypes.RecommendedResources{
		TargetPodNum:             podRecommendation.TargetPodNum,
		LowerBoundPodNum:         podRecommendation.LowerBoundPodNum,
		UpperBoundPodNum:         podRecommendation.UpperBoundPodNum,
		UncappedTargetPodNum:     podRecommendation.UncappedTargetPodNum,
		ContainerRecommendations: adjustedRecommendations,
	}, containersAnnotations, nil
}

// GetContainerRecommendation 获取指定容器的推荐资源
func GetContainerRecommendation(
	containerName string,
	recommendation []mpaTypes.RecommendedContainerResources) *mpaTypes.RecommendedContainerResources {
	if recommendation != nil {
		for _, recomm := range recommendation {
			if containerName == recomm.ContainerName {
				return &recomm
			}
		}
	}
	return nil
}

// adjustRecommendationForContainer 调整容器推荐资源使其符合 mpa policy 和 limit range 的限制
func adjustRecommendationForContainer(
	container corev1.Container,
	recommendation *mpaTypes.RecommendedContainerResources,
	podPolicy *mpaTypes.PodResourcePolicy,
	limitRange *corev1.LimitRangeItem,
) (*mpaTypes.RecommendedContainerResources, []string, error) {
	if recommendation == nil {
		return nil, nil, fmt.Errorf("no recommendation aviliable for container: %v", container.Name)
	}

	containerPolicy := mpaApi.GetContainerResourcePolicy(container.Name, podPolicy)
	adjustedRecommendation := recommendation.DeepCopy()

	adjustAnnotations := make([]string, 0)

	process := func(recomm corev1.ResourceList, getAnnotations bool) {
		limitAnnotations := adjustToContainerLimitRange(recomm, container, limitRange)
		policyAnnotations := adjustToMpaPolicy(recomm, containerPolicy)
		if getAnnotations {
			adjustAnnotations = append(adjustAnnotations, limitAnnotations...)
			adjustAnnotations = append(adjustAnnotations, policyAnnotations...)
		}
	}
	process(adjustedRecommendation.Target, true)
	process(adjustedRecommendation.UpperBound, false)
	process(adjustedRecommendation.LowerBound, false)

	return adjustedRecommendation, adjustAnnotations, nil
}

// adjustAnnotation 返回一个 annotation 标记
func adjustAnnotation(resourceName corev1.ResourceName, action adjustAction) string {
	return fmt.Sprintf("%s:%s", resourceName, action)
}

func adjustToMpaPolicy(
	recommendation corev1.ResourceList,
	policy *mpaTypes.ContainerResourcePolicy) []string {
	if policy == nil {
		return nil
	}
	annotations := make([]string, 0)

	for name, recommened := range recommendation {
		// 调整下限
		toMin, overflow := adjustToPolicyMin(name, recommened, *policy)
		recommendation[name] = toMin
		if overflow {
			annotations = append(annotations, adjustAnnotation(name, adjustToMinAllowed))
		}
		// 调整上限
		toMax, overflow := adjustToPolicyMax(name, recommened, *policy)
		recommendation[name] = toMax
		if overflow {
			annotations = append(annotations, adjustAnnotation(name, adjustToMaxAllowed))
		}
	}
	return annotations
}

func adjustToContainerLimitRange(
	recommendation corev1.ResourceList,
	container corev1.Container,
	limitRrange *corev1.LimitRangeItem,
) []string {
	// TODO: 调整容器资源推荐方案以符合容器 LimitRange 的限制
	return make([]string, 0)
}

func (p *processor) adjustToPodLimitRange(
	recommendations []mpaTypes.RecommendedContainerResources,
	pod *corev1.Pod,
) ([]mpaTypes.RecommendedContainerResources, error) {
	podLimitRange, err := p.limitRangeCalculator.GetPodLimitRangeItem(pod.Namespace)
	if err != nil {
		return nil, fmt.Errorf("connot fetch pod(name: %v)'s limit range: %v", pod.Name, err)
	}
	if podLimitRange == nil {
		// 没有 limit range 的限制，原样返回
		return recommendations, nil
	}
	// TODO: 调整推荐以符合 POD 的 LimitRange 限制
	return recommendations, nil
}

// adjustToPolicyMin 调整 推荐资源量 符合policy策略的最小值
func adjustToPolicyMin(
	resourceName corev1.ResourceName,
	recommended resource.Quantity, policy mpaTypes.ContainerResourcePolicy,
) (resource.Quantity, bool) {
	return adjustToMin(resourceName, recommended, policy.MinAllowed)
}

// adjustToPolicyMax 调整 推荐资源量 符合policy策略的最大值
func adjustToPolicyMax(
	resourceName corev1.ResourceName,
	recommended resource.Quantity, policy mpaTypes.ContainerResourcePolicy,
) (resource.Quantity, bool) {
	return adjustToMax(resourceName, recommended, policy.MaxAllowed)
}

// adjustToMin 判断 recommended 是否小于 minAllowed 资源量
// 1. 如果小于, 返回 minAllowed
// 2. 大于等于, 返回 recommended (不变)
func adjustToMin(
	resourceName corev1.ResourceName,
	recommended resource.Quantity, min corev1.ResourceList,
) (resource.Quantity, bool) {
	minQuantity, exists := min[resourceName]
	if exists && !minQuantity.IsZero() && recommended.Cmp(minQuantity) < 0 {
		return minQuantity, true
	}
	return recommended, false
}

// adjustToMax 判断 recommended 是否超出了 maxAllowed 资源量
// 1. 如果超出, 返回 maxAllowed
// 2. 未超出, 返回 recommended (不变)
func adjustToMax(
	resourceName corev1.ResourceName,
	recommended resource.Quantity, max corev1.ResourceList,
) (resource.Quantity, bool) {
	maxQuantity, exists := max[resourceName]
	if exists && !maxQuantity.IsZero() && recommended.Cmp(maxQuantity) > 0 {
		return maxQuantity, true
	}
	return recommended, false
}

// getContainer 获取指定 pod 中定义的 container 对象
// 未找到返回 nil
func getContainer(containerName string, pod *corev1.Pod) *corev1.Container {
	if pod != nil {
		for _, container := range pod.Spec.Containers {
			if containerName == container.Name {
				return &container
			}
		}
	}
	return nil
}
