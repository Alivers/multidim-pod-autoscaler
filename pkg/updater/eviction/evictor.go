package eviction

import (
	corev1 "k8s.io/api/core/v1"
	kubeClient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"multidim-pod-autoscaler/pkg/target"
	"time"
)

const (
	// informer 缓存的同步时间间隔
	defaultResyncPeriod time.Duration = 1 * time.Minute
)

// PodEvictor 驱逐指定pod
// 生命周期为一个updater主流程执行过程
// 即每次updater run一次都会创建新的 PodEvictor
type PodEvictor interface {
	// Evict 驱逐指定pod，并上报event(使用eventRecorder)
	Evict(pod *corev1.Pod, eventRecorder record.EventRecorder) error
	// Evictable 判断指定pod是否可以被驱逐
	Evictable(pod *corev1.Pod) bool
}

// podManagedController 为管理 pod 的controller信息
// 如: replicaset、statefulset等
type podManagedController struct {
	Namespace string
	Name      string
	Kind      target.WellKnownController
}

// managingControllerStates 描述了 controller 中的pod副本的数量状态
// configured - controller中预配置的副本个数
// pending - pending 状态的个数
// running - running 状态的个数
// evicted - 被驱逐的pod的个数
// evictable - 可以被驱逐的数量(configured * [rate])
type managingControllerStates struct {
	configured int
	pending    int
	running    int
	evicted    int
	evictable  int
}

type podEvictor struct {
	client kubeClient.Interface
}
