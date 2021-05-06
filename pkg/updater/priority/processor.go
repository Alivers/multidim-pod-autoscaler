package priority

import (
	corev1 "k8s.io/api/core/v1"
	mpaTypes "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"
)

// Processor 处理pods的更新优先级, 返回按更新顺序排列的pods列表
type Processor interface {
	// GetPodsUpdateOrder 获取指定pods & mpa 下，pods的更新顺序
	GetPodsUpdateOrder(pod []*corev1.Pod, mpa *mpaTypes.MultidimPodAutoscaler) []*corev1.Pod
}

// TODO: 实现pod的更新优先级排队
type processor struct {
}

func NewProcessor() Processor {
	return &processor{}
}

func (p *processor) GetPodsUpdateOrder(pod []*corev1.Pod, mpa *mpaTypes.MultidimPodAutoscaler) []*corev1.Pod {
	// panic("implement me")
	return pod
}
