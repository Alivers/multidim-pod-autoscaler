package api

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
	mpaTypes "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"
	lister "multidim-pod-autoscaler/pkg/client/listers/autoscaling/v1"
	"multidim-pod-autoscaler/pkg/target"
)

// Matcher 返回与给定pod匹配的MPA Object
type Matcher interface {
	GetPodMatchingMpa(pod *corev1.Pod) *mpaTypes.MultidimPodAutoscaler
}

// matcher implements the Matcher.GetPodMatchingMpa interface
type matcher struct {
	mpaLister       lister.MultidimPodAutoscalerLister
	selectorFetcher target.MpaTargetSelectorFetch
}

// NewMatcher 返回一个新的Matcher
func NewMatcher(mpaLister lister.MultidimPodAutoscalerLister, selectorFetcher target.MpaTargetSelectorFetch) Matcher {
	return &matcher{
		mpaLister:       mpaLister,
		selectorFetcher: selectorFetcher,
	}
}

// GetPodMatchingMpa 返回与给定pod匹配的MPA Object
func (m *matcher) GetPodMatchingMpa(pod *corev1.Pod) *mpaTypes.MultidimPodAutoscaler {
	// 获取pod同一命名空间下的所有 MPA Object
	mpas, err := m.mpaLister.MultidimPodAutoscalers(pod.Namespace).List(labels.Everything())
	if err != nil {
		klog.Errorf("Failed to get MPA objects from lister: %v", err)
	}

	mpasWithSelector := make([]*MpaWithSelector, 0)
	for _, mpa := range mpas {
		// 如果该MPA不需要更新，跳过
		if GetMpaUpdateMode(mpa) == mpaTypes.UpdateModeOff {
			continue
		}
		// 获取MPA Objector 对应的 label Selector
		selector, err := m.selectorFetcher.Fetch(mpa)
		if err != nil {
			klog.V(3).Infof("Cannot fetch selector of %v, skipped: %s", mpa.Name, err)
			continue
		}
		// 加入到待选 MPA 中
		mpasWithSelector = append(mpasWithSelector,
			&MpaWithSelector{
				Mpa:      mpa,
				Selector: selector,
			},
		)
	}
	// 选择匹配的MPA Object
	targetMpa := GetControllingMpaForPod(pod, mpasWithSelector)

	if targetMpa != nil {
		return targetMpa.Mpa
	}
	return nil
}
