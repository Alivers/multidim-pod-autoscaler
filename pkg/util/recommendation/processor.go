package recommendation

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	mpaTypes "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"
	limitrange "multidim-pod-autoscaler/pkg/util/limitrange"
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

func (p *processor) AdjustRecommendation(
	podRecommendation *mpaTypes.RecommendedResources,
	policy *mpaTypes.PodResourcePolicy,
	pod *corev1.Pod,
) (*mpaTypes.RecommendedResources, ContainerAnnotationsMap, error) {
	panic("implement me")
}

func adjustRecommendationForContainer(
	container corev1.Container,
	recommendation *mpaTypes.RecommendedResources,
) {

}

// adjustAnnotation 返回一个 annotation 标记
func adjustAnnotation(resourceName corev1.ResourceName, action adjustAction) string {
	return fmt.Sprintf("%s:%s", resourceName, action)
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
