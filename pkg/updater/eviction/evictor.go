package eviction

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeClient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"multidim-pod-autoscaler/pkg/target"
	updaterUtil "multidim-pod-autoscaler/pkg/updater/util"
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

// PodEvictorFactory 创建新的 PodEvictor
type PodEvictorFactory interface {
	NewPodEvictor(pods []*corev1.Pod) PodEvictor
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

// podEvictor 实现 PodEvictor 接口
// podControllerMap 为 pod -> pod's OwnerRef 的map
// controllerStatesMap 为 pod's OwnerRef(Controller) -> Controller's states 的map
type podEvictor struct {
	client              kubeClient.Interface
	podControllerMap    map[string]podManagedController
	controllerStatesMap map[podManagedController]managingControllerStates
}

// podEvictorFactory 包含了创建 podEvictor 的必要信息
// informersMap 为控制器 -> Informer 的map
// minReplicasToUpdate 为可被更新的最少的副本数量
// evictionFraction 表示最多可驱逐的pod的比例(相对于预配置的replicas的比例)
type podEvictorFactory struct {
	client              kubeClient.Interface
	informersMap        map[target.WellKnownController]cache.SharedIndexInformer
	minReplicasToUpdate int
	evictionFraction    float64
}

// NewPodEvictorFactory 返回PodEvictorFactory
func NewPodEvictorFactory(client kubeClient.Interface, minReplicasToUpdate int, evictionFraction float64) (PodEvictorFactory, error) {
	replicaSetInformer, err := updaterUtil.GetInformer(client, target.ReplicaSet, defaultResyncPeriod)
	if err != nil {
		return nil, fmt.Errorf("failed to create ReplicaSet informer: %v", err)
	}
	replicaControllerInformer, err := updaterUtil.GetInformer(client, target.ReplicationController, defaultResyncPeriod)
	if err != nil {
		return nil, fmt.Errorf("failed to create ReplicationController informer: %v", err)
	}
	statefulSetInformer, err := updaterUtil.GetInformer(client, target.StatefulSet, defaultResyncPeriod)
	if err != nil {
		return nil, fmt.Errorf("failed to create StatefulSet informer: %v", err)
	}

	return &podEvictorFactory{
		client: client,
		informersMap: map[target.WellKnownController]cache.SharedIndexInformer{
			target.ReplicaSet:            replicaSetInformer,
			target.ReplicationController: replicaControllerInformer,
			target.StatefulSet:           statefulSetInformer,
		},
	}, nil
}

// NewPodEvictor 创建一个新的 PodEvictor
func (factory *podEvictorFactory) NewPodEvictor(pods []*corev1.Pod) PodEvictor {
	controllerPods := make(map[podManagedController][]*corev1.Pod)
	// 获取每个controller管理的pod集合
	for _, pod := range pods {
		controller, err := getPodManagedController(pod)
		if err != nil {
			klog.Error(err)
			continue
		}
		controllerPods[*controller] = append(controllerPods[*controller], pod)
	}

	podControllerMap := make(map[string]podManagedController)
	controllerStatesMap := make(map[podManagedController]managingControllerStates)

	for controller, pods := range controllerPods {
		// 实际的副本个数
		actualReplicas := len(pods)
		if actualReplicas < factory.minReplicasToUpdate {
			klog.V(2).Infof("too few replicas to Execute MPA Update for %s controller(%s/%s)", controller.Kind, controller.Namespace, controller.Name)
			continue
		}
		var err error
		var configured int

		// 获取预配置的replicas
		if controller.Kind == target.Job {
			configured = actualReplicas
		} else {
			informer, exists := factory.informersMap[controller.Kind]
			if !exists {
				klog.V(4).Infof("cannot found the informer of %s Controller(%s/%s)", controller.Kind, controller.Namespace, controller.Name)
				continue
			}
			configured, err = updaterUtil.GetControllerReplicaCount(controller.Namespace, controller.Name, controller.Kind, informer)
			if err != nil {
				klog.Errorf("failed to fetch replicas configuration for %v %s/%s: %v", controller.Kind, controller.Namespace, controller.Name, err)
				continue
			}
		}
		// 统计该controller的副本状态信息
		controllerStates := managingControllerStates{
			configured: configured,
			evictable:  int(float64(configured) * factory.evictionFraction),
		}
		// 为该controller下所有副本注册controller引用信息
		for _, pod := range pods {
			podControllerMap[updaterUtil.GetPodId(pod)] = controller
			if pod.Status.Phase == corev1.PodPending {
				controllerStates.pending += 1
			}
		}
		// 默认 running = 实际副本个数 - pending
		controllerStates.running = actualReplicas - controllerStates.pending
		// 存储controller的副本状态信息
		controllerStatesMap[controller] = controllerStates
	}

	return &podEvictor{
		client:              factory.client,
		podControllerMap:    podControllerMap,
		controllerStatesMap: controllerStatesMap,
	}
}

// Evict 驱逐指定pod，并上报event(使用eventRecorder)
// 不会检查处于驱逐宽限期的pod状态
func (evictor *podEvictor) Evict(pod *corev1.Pod, eventRecorder record.EventRecorder) error {
	controller, exists := evictor.podControllerMap[updaterUtil.GetPodId(pod)]
	if !exists {
		return fmt.Errorf("does found the owner controller of pod(%s/%s), cannot evict the pod", pod.Namespace, pod.Name)
	}

	if !evictor.Evictable(pod) {
		return fmt.Errorf("cannot evict pod(%s/%s) for its owner controller", pod.Namespace, pod.Name)
	}

	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		},
	}

	err := evictor.client.CoreV1().Pods(pod.Namespace).Evict(context.TODO(), eviction)
	if err != nil {
		klog.Errorf("failed to evict pod %s/%s, error: %v", pod.Namespace, pod.Name, err)
		return err
	}
	// pending状态的pod不计数
	if pod.Status.Phase != corev1.PodPending {
		controllerStates, exists := evictor.controllerStatesMap[controller]
		if !exists {
			return fmt.Errorf("cannot find states for replication group %v", controller)
		}
		controllerStates.evicted += 1
		evictor.controllerStatesMap[controller] = controllerStates
	}

	return nil
}

// Evictable 判断指定pod是否可以被驱逐
func (evictor *podEvictor) Evictable(pod *corev1.Pod) bool {
	controller, exists := evictor.podControllerMap[updaterUtil.GetPodId(pod)]
	if exists {
		// 找到了管理该pod的controller，且该pod处理pending中，
		// 可以直接驱逐
		if pod.Status.Phase == corev1.PodPending {
			return true
		}

		controllerStates, exists := evictor.controllerStatesMap[controller]
		if exists {
			// 可驱逐数量足够
			aliveNum := controllerStates.configured - controllerStates.evictable
			if controllerStates.running-controllerStates.evicted > aliveNum {
				return true
			}
			// 所有pod都在运行状态, 且 evictFraction 很小(使得evictable为0), 且当前为驱逐其他pod
			// 此时可以驱逐一个pod(再次回到这里会不满足 evicted == 0 而不会驱逐其他pod)
			// (在这之前已经对可update的最小副本数量做了限制)
			if controllerStates.running == controllerStates.configured &&
				controllerStates.evictable == 0 &&
				controllerStates.evicted == 0 {
				return true
			}
		}
	}

	return false
}

func getPodManagedController(pod *corev1.Pod) (*podManagedController, error) {
	ownerRef := updaterUtil.GetPodManagedControllerRef(pod)
	if ownerRef == nil {
		return nil, fmt.Errorf("connot found the ownerReference(points to Controller) for pod(%s)", pod.Name)
	}
	controller := &podManagedController{
		Namespace: pod.Namespace,
		Name:      ownerRef.Name,
		Kind:      target.WellKnownController(ownerRef.Kind),
	}
	return controller, nil
}
