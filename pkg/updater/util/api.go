package util

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	kubeClient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"multidim-pod-autoscaler/pkg/target"
	"time"
)

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

// GetPodId 获取pod的id，"[namespace]/[name]"
func GetPodId(pod *corev1.Pod) string {
	if pod == nil {
		return ""
	}
	return pod.Namespace + "/" + pod.Name
}
