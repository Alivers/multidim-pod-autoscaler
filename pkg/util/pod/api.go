package pod

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	kubeClient "k8s.io/client-go/kubernetes"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"time"
)

// NewPodLister 获取指定 namespace 下的 podLister
func NewPodLister(kubeclient kubeClient.Interface, namespace string, stopCh <-chan struct{}) listers.PodLister {
	selector := fields.ParseSelectorOrDie("spec.nodeName!=" + "" + ",status.phase!=" +
		string(corev1.PodSucceeded) + ",status.phase!=" + string(corev1.PodFailed))

	podListWatch := cache.NewListWatchFromClient(kubeclient.CoreV1().RESTClient(), "pods", namespace, selector)
	store := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc,
		cache.Indexers{
			cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
		},
	)

	podLister := listers.NewPodLister(store)
	podReflector := cache.NewReflector(podListWatch, &corev1.Pod{}, store, time.Hour)

	go podReflector.Run(stopCh)

	return podLister
}
