package util

import (
	"fmt"
	appsV1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	kubeClient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"

	"multidim-pod-autoscaler/pkg/target"
	"time"
)

// NewEventRecorder 返回一个新的 EvenetRecorder 用于事件上报
// component 为上报事件的组件名
func NewEventRecorder(client kubeClient.Interface, component string) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.V(4).Infof)
	if _, isFake := client.(*fake.Clientset); !isFake {
		eventBroadcaster.StartRecordingToSink(
			&clientv1.EventSinkImpl{
				Interface: clientv1.New(client.CoreV1().RESTClient()).Events(""),
			},
		)
	}
	return eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: component})
}

// GetInformer 启动一个指定 controller kind 的informer
// (使用 shared informer factory 创建)
func GetInformer(
	kubeclient kubeClient.Interface,
	controllerKind target.WellKnownController,
	resyncPeriod time.Duration,
) (cache.SharedIndexInformer, error) {

	var informer cache.SharedIndexInformer
	factory := informers.NewSharedInformerFactory(kubeclient, resyncPeriod)

	switch controllerKind {
	case target.ReplicaSet:
		informer = factory.Apps().V1().ReplicaSets().Informer()
	case target.ReplicationController:
		informer = factory.Core().V1().ReplicationControllers().Informer()
	case target.StatefulSet:
		informer = factory.Apps().V1().StatefulSets().Informer()
	default:
		return nil, fmt.Errorf("unsupported controller kind: %s", controllerKind)
	}
	// 运行informer并等待local store缓存完成
	stopCh := make(chan struct{})
	go informer.Run(stopCh)
	if synced := cache.WaitForCacheSync(stopCh, informer.HasSynced); !synced {
		return nil, fmt.Errorf("failed to sync %s store", controllerKind)
	}

	return informer, nil
}

// GetPodManagedControllerRef 获取指定pod的属主(且该属主指向的是Controller)
func GetPodManagedControllerRef(pod *corev1.Pod) *metaV1.OwnerReference {
	var managingController metaV1.OwnerReference

	for _, owerRef := range pod.OwnerReferences {
		if *owerRef.Controller {
			managingController = owerRef
		}
	}

	return &managingController
}

// GetControllerReplicaCount 获取指定controller的replicas配置
// informer 为对应controller的informer
func GetControllerReplicaCount(
	controllerNamespace, controllerName string,
	controllerKind target.WellKnownController,
	informer cache.SharedIndexInformer) (int, error) {

	// 在store中获取对象
	itemObj, exists, err := informer.GetStore().GetByKey(controllerNamespace + "/" + controllerName)
	if err != nil {
		return 0, fmt.Errorf("failed to get %s controller Ojbect(%s/%s): %v", controllerKind, controllerNamespace, controllerName, err)
	}
	if !exists {
		return 0, fmt.Errorf("%s controller Object(%s/%s) does not exists", controllerKind, controllerNamespace, controllerName)
	}
	// 根据kind解析为不同的controller
	// 暂时只支持下面三种controller kind
	switch controllerKind {
	case target.ReplicaSet:
		replicaSet, ok := itemObj.(*appsV1.ReplicaSet)
		if !ok {
			return 0, fmt.Errorf("failed to parse replicaSet controller(%s/%s)", controllerNamespace, controllerName)
		}
		if replicaSet.Spec.Replicas == nil || *replicaSet.Spec.Replicas == 0 {
			return 0, fmt.Errorf("replicaSet controller(%s/%s) has no replicas configuration", controllerNamespace, controllerName)
		}
		return int(*replicaSet.Spec.Replicas), nil
	case target.ReplicationController:
		replication, ok := itemObj.(*corev1.ReplicationController)
		if !ok {
			return 0, fmt.Errorf("failed to parse replication controller(%s/%s)", controllerNamespace, controllerName)
		}
		if replication.Spec.Replicas == nil || *replication.Spec.Replicas == 0 {
			return 0, fmt.Errorf("replication controller(%s/%s) has no replicas configuration", controllerNamespace, controllerName)
		}
		return int(*replication.Spec.Replicas), nil
	case target.StatefulSet:
		statefulSet, ok := itemObj.(*appsV1.StatefulSet)
		if !ok {
			return 0, fmt.Errorf("failed to parse statefulSet controller(%s/%s)", controllerNamespace, controllerName)
		}
		if statefulSet.Spec.Replicas == nil || *statefulSet.Spec.Replicas == 0 {
			return 0, fmt.Errorf("statefulSet controller(%s/%s) has no replicas configuration", controllerNamespace, controllerName)
		}
		return int(*statefulSet.Spec.Replicas), nil
	}

	return 0, fmt.Errorf("unsupported controller kind(%s) for now to get its replica count", controllerKind)
}

// GetPodId 获取pod的id，"[namespace]/[name]"
func GetPodId(pod *corev1.Pod) string {
	if pod == nil {
		return ""
	}
	return pod.Namespace + "/" + pod.Name
}
