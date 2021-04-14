package api

import (
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	mpaTypes "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"
	clientset "multidim-pod-autoscaler/pkg/client/clientset/versioned"
	lister "multidim-pod-autoscaler/pkg/client/listers/autoscaling/v1"
	"time"
)

// MpaWithSelector mpa 和其对应的 selector
type MpaWithSelector struct {
	Mpa      *mpaTypes.MultidimPodAutoscaler
	Selector labels.Selector
}

// NewMpasLister 返回 MPA lister(获取指定命名空间下的所有MPA Object)
func NewMpasLister(
	mpaClient *clientset.Clientset,
	namespace string,
	stopChan <-chan struct{}) lister.MultidimPodAutoscalerLister {
	// 创建 lister(全量资源) watcher(资源增量) 来获取资源信息
	mpaListWatch := cache.NewListWatchFromClient(mpaClient.AutoscalingV1().RESTClient(),
		"multidimpodautoscalers", namespace, fields.Everything())

	// indexer 用来索引缓存中的资源
	indexer, controller := cache.NewIndexerInformer(
		mpaListWatch,
		&mpaTypes.MultidimPodAutoscaler{},
		1*time.Hour,
		&cache.ResourceEventHandlerFuncs{},
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
	// 创建 mpa lister
	mpaLister := lister.NewMultidimPodAutoscalerLister(indexer)

	// controller 启动 reflector(通过lister watcher获取资源数据并存入DeltaFIFO, 最终存储到local store)
	go controller.Run(stopChan)
	// 等待本地缓存填充完成
	if !cache.WaitForCacheSync(make(chan struct{}), controller.HasSynced) {
		klog.Fatalf("Failed to sync MPA cache during initialization")
	} else {
		klog.Infof("Initial MPA synced successful")
	}

	return mpaLister
}

// GetMpaUpdateMode 获取指定MPA的 updatePolicy.UpdateMode (资源更新策略)
func GetMpaUpdateMode(mpa *mpaTypes.MultidimPodAutoscaler) mpaTypes.UpdateMode {
	// 如果未指定，返回默认模式：UpdateModeAuto
	if mpa.Spec.UpdatePolicy == nil || mpa.Spec.UpdatePolicy.UpdateMode == nil || *mpa.Spec.UpdatePolicy.UpdateMode == "" {
		return mpaTypes.UpdateModeAuto
	}
	return *mpa.Spec.UpdatePolicy.UpdateMode
}
