package limitrange

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	listers "k8s.io/client-go/listers/core/v1"
)

// Calculator 计算容器或POD的资源量限制
type Calculator interface {
	// GetContainerLimitRangeItem 获取指定命令空间下的容器资源量限制
	GetContainerLimitRangeItem(namespace string) (*corev1.LimitRangeItem, error)
	// GetPodLimitRangeItem 获取指定命名空间下的POD资源量限制
	GetPodLimitRangeItem(namespace string) (*corev1.LimitRangeItem, error)
}

type limitCalculator struct {
	limitRangeLister listers.LimitRangeLister
}

// GetContainerLimitRangeItem 获取指定命令空间下的容器资源量限制
func (l *limitCalculator) GetContainerLimitRangeItem(namespace string) (*corev1.LimitRangeItem, error) {
	return l.getLimitRangeItem(namespace, corev1.LimitTypeContainer)
}

// GetPodLimitRangeItem 获取指定命名空间下的POD资源量限制
func (l *limitCalculator) GetPodLimitRangeItem(namespace string) (*corev1.LimitRangeItem, error) {
	return l.getLimitRangeItem(namespace, corev1.LimitTypePod)
}

// NewCalculator 返回一个新的 limit range Calculator
func NewCalculator(factory informers.SharedInformerFactory) (Calculator, error) {
	if factory == nil {
		return nil, fmt.Errorf("NewLimitRangeCalculator required a SharedInformerFactory but got nil")
	}
	limitRangeLister := factory.Core().V1().LimitRanges().Lister()

	// 需要等待informer的store中同步得到limitrange 数据
	stopCh := make(chan struct{})
	factory.Start(stopCh)

	for _, ok := range factory.WaitForCacheSync(stopCh) {
		if !ok && !factory.Core().V1().LimitRanges().Informer().HasSynced() {
			return nil, fmt.Errorf("infromer did not synced")
		}
	}
	return &limitCalculator{limitRangeLister: limitRangeLister}, nil
}

func (l *limitCalculator) getLimitRangeItem(namespace string, resourceType corev1.LimitType) (*corev1.LimitRangeItem, error) {
	limitRanges, err := l.limitRangeLister.LimitRanges(namespace).List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("cannot loading limitRanges from namespace(%v): %v", namespace, err)
	}

	targetLimitRangeItem := &corev1.LimitRangeItem{Type: resourceType}

	for _, limitRange := range limitRanges {
		for _, limitItem := range limitRange.Spec.Limits {
			if limitItem.Type == resourceType && (limitItem.Min != nil || limitItem.Max != nil || limitItem.Default != nil) {
				if limitItem.Default != nil {
					targetLimitRangeItem.Default = limitItem.Default
				}
				// 更新 CPU 的最大下界
				targetLimitRangeItem.Min = updateResource(targetLimitRangeItem.Min, limitItem.Min, corev1.ResourceCPU, chooseMaxLowerBound)
				// 更新 memory 的最大下界
				targetLimitRangeItem.Min = updateResource(targetLimitRangeItem.Min, limitItem.Min, corev1.ResourceMemory, chooseMaxLowerBound)
				// 更新 CPU 的最小上界
				targetLimitRangeItem.Max = updateResource(targetLimitRangeItem.Max, limitItem.Max, corev1.ResourceCPU, chooseMinUpperBound)
				// 更新 memory 的最小上界
				targetLimitRangeItem.Max = updateResource(targetLimitRangeItem.Max, limitItem.Max, corev1.ResourceMemory, chooseMinUpperBound)
			}
		}
	}
	if targetLimitRangeItem.Max != nil || targetLimitRangeItem.Min != nil || targetLimitRangeItem.Default != nil {
		return targetLimitRangeItem, nil
	}
	return nil, nil
}

// updateResource 更新dst的资源配额
// 使用selector自定义选择dst和src中更合适的资源(resourceName)
func updateResource(dst, src corev1.ResourceList,
	resourceName corev1.ResourceName,
	selector func(a, b resource.Quantity) resource.Quantity) corev1.ResourceList {
	if src == nil {
		return dst
	}
	if dst == nil {
		return src.DeepCopy()
	}

	if srcResource, srcOk := src[resourceName]; srcOk {
		dstResource, dstOk := dst[resourceName]
		if dstOk {
			dst[resourceName] = selector(dstResource, srcResource)
		} else {
			dst[resourceName] = srcResource.DeepCopy()
		}
	}

	return dst
}

// chooseMinUpperBound 选择资源上界(选择最小的上界)
// 满足所有资源约束
func chooseMinUpperBound(a, b resource.Quantity) resource.Quantity {
	if a.Cmp(b) < 0 {
		return a
	}
	return b
}

// chooseMaxLowerBound 选择资源下界(选择最大的下界)
// 满足所有资源约束
func chooseMaxLowerBound(a, b resource.Quantity) resource.Quantity {
	if a.Cmp(b) > 0 {
		return a
	}
	return b
}
