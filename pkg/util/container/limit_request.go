package container

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"math"
	"math/big"
)

// Resources 保存了容器的资源配额
// 路径(.resources.limits .resources.requests)
type Resources struct {
	Limits   corev1.ResourceList
	Requests corev1.ResourceList
}

type roundType int

const (
	noRound roundType = iota
	roundUp
	roundDown
)

// GetProportionalLimit 获取资源recommended Limit
// 通过缩放 originalLimit 获取, 缩放比例为(recommendedRequest:originalRequest)
// 返回 (recommednedLimit, annotations)
// annotations 为获取过程中的一些必要信息(独属于某一个容器)
func GetProportionalLimit(originalLimit, originalRequest,
	recommendedRequest,
	defaultLimit corev1.ResourceList) (corev1.ResourceList, []string) {
	annotations := make([]string, 0)
	// 计算CPU recommended limit
	cpuLimit, annotation := getProportionalResourceLimit(
		corev1.ResourceCPU,
		originalLimit.Cpu(), originalRequest.Cpu(),
		recommendedRequest.Cpu(), defaultLimit.Cpu(),
	)
	if annotation != "" {
		annotations = append(annotations, annotation)
	}
	// 计算memory recommended limit
	memoryLimit, annotation := getProportionalResourceLimit(
		corev1.ResourceMemory,
		originalLimit.Memory(), originalRequest.Memory(),
		recommendedRequest.Memory(), defaultLimit.Memory(),
	)

	if annotation != "" {
		annotations = append(annotations, annotation)
	}

	result := corev1.ResourceList{}
	if cpuLimit != nil {
		result[corev1.ResourceCPU] = *cpuLimit
	}
	if memoryLimit != nil {
		result[corev1.ResourceMemory] = *memoryLimit
	}
	return result, annotations
}

// GetBoundaryRequest 获取 Max/Min(boundary) Request
// 通过缩放 originalRequest 获取, 缩放比例为 (boundaryLimit : originalLimit)
func GetBoundaryRequest(originalRequest, originalLimit, boundaryLimit, defaultLimit *resource.Quantity) *resource.Quantity {
	// 1. originalLimit 未设置且指定了 default limit时，originalLimit与 defaultLimit 相等
	if (originalLimit == nil || originalLimit.Value() == 0) && defaultLimit != nil {
		originalLimit = defaultLimit
	}
	// 2. 这里表示 originalLimit 未被指定，且 defaultLimit为nil(或value等于0)
	// 直接返回 空quantiy
	if originalLimit == nil || originalLimit.Value() == 0 {
		return &resource.Quantity{}
	}

	// 3. originalRequest 未指定, 应与 limit 相同
	if originalRequest == nil || originalRequest.Value() == 0 {
		result := *boundaryLimit
		return &result
	}
	// 伸缩originalRequest(比例为 boundaryLimit:originalLimit)
	result, _ := scaleQuantityProportionally(originalRequest, originalLimit, boundaryLimit, noRound)
	return result
}

// getProportionalResourceLimit 获取 originalLimit 缩放后的 Limit
// 缩放比例为 recommednedRequest : originalRequest
// 即缩放后保证:
// recommendedLimit(返回值) : oringinalLimit == recommendedRequest : originalRequest
// (需要处理一些边界情况)
// 返回的字符串作为
func getProportionalResourceLimit(resourceName corev1.ResourceName,
	originalLimit, originalRequest, recommendedRequest, defaultLimit *resource.Quantity) (*resource.Quantity, string) {
	// 1. originalLimit 未设置且指定了 default limit时，originalLimit与 defaultLimit 相等
	if (originalLimit == nil || originalLimit.Value() == 0) && defaultLimit != nil {
		originalLimit = defaultLimit
	}

	// 2. 这里表示 originalLimit 未被指定，且 defaultLimit为nil(或value等于0)
	// 直接返回 nil
	if originalLimit == nil || originalLimit.Value() == 0 {
		return nil, ""
	}
	// 3. 当 recommendedRequest 为空时, 其对应的 recommendedLimit 也应为空
	if recommendedRequest == nil || recommendedRequest.Value() == 0 {
		return nil, ""
	}
	// 4. 当 originalRequest 未被指定时, recommededLimit 应与 recommendedRequest 相等
	if originalRequest == nil || originalLimit.Value() == 0 {
		result := *recommendedRequest
		return &result, ""
	}

	// 5. 当 originalLimit 与 originalRequest 相等时, 伸缩比例为1:1, 不需要伸缩
	if originalLimit.MilliValue() == originalRequest.MilliValue() {
		result := *recommendedRequest
		return &result, ""
	}

	result, overflow := scaleQuantityProportionally(originalLimit, originalRequest, recommendedRequest, noRound)
	if !overflow {
		return result, ""
	}
	return result,
		fmt.Sprintf("%v: failed to scale limit to the same ration of request; capping limit to Int64", resourceName)
}

// scaleQuantityProportionally 按比例(baseScaled : base)缩放 scaling, 返回 result
// 即 result : scaling == baseScaled : base
// 当scale后超出Int64的表示范围, 返回值 bool 为 true; 未超出时始终返回 false
func scaleQuantityProportionally(scaling, base, baseScaled *resource.Quantity, round roundType) (*resource.Quantity, bool) {
	scalingMilli := big.NewInt(scaling.MilliValue())
	baseMilli := big.NewInt(base.MilliValue())
	baseScaledMilli := big.NewInt(baseScaled.MilliValue())

	var scaledMilli big.Int
	scaledMilli.Mul(scalingMilli, baseScaledMilli)
	scaledMilli.Div(&scaledMilli, baseMilli)

	if scaledMilli.IsInt64() {
		scaled := resource.NewMilliQuantity(scaledMilli.Int64(), scaling.Format)
		if round == roundUp {
			// 向上对对齐到一个完整资源
			scaled.RoundUp(resource.Scale(0))
		} else if round == roundDown {
			// 向上对对齐到一个完整资源; 先减去(1000-1), 保证向下对齐了一个单位
			scaled.Sub(*resource.NewMilliQuantity(999, scaled.Format))
			scaled.RoundUp(resource.Scale(0))
		}
		return scaled, false
	}

	return resource.NewMilliQuantity(math.MaxInt64, scaling.Format), true
}
