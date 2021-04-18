package api

import (
	"context"
	"encoding/json"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	mpaTypes "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"
	clientset "multidim-pod-autoscaler/pkg/client/clientset/versioned"
	clientType "multidim-pod-autoscaler/pkg/client/clientset/versioned/typed/autoscaling/v1"
	lister "multidim-pod-autoscaler/pkg/client/listers/autoscaling/v1"
	"multidim-pod-autoscaler/pkg/util/patch"
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

// GetContainerResourcePolicy 获取指定容器的资源策略
// 返回值可能为 nil
func GetContainerResourcePolicy(containerName string, podPolicy *mpaTypes.PodResourcePolicy) *mpaTypes.ContainerResourcePolicy {
	var defaultPolicy *mpaTypes.ContainerResourcePolicy

	if podPolicy != nil {
		for i, containerPolicy := range podPolicy.ContainerPolicies {
			// 先严格匹配容器名称
			if containerPolicy.ContainerName == containerName {
				return &podPolicy.ContainerPolicies[i]
			}
			// 未匹配到指定容器名，但包含通配容器名，该容器资源策略直接应用到全部容器
			if containerPolicy.ContainerName == mpaTypes.DefaultContainerResourcePolicy {
				defaultPolicy = &podPolicy.ContainerPolicies[i]
			}
		}
	}
	return defaultPolicy
}

// GetContainerControlledMode 获取容器 request limit 的控制方式
// 默认为 request limit 同时控制(按比例伸缩)
func GetContainerControlledMode(containerName string, podPolicy *mpaTypes.PodResourcePolicy) mpaTypes.ContainerControlledMode {
	containerPolicy := GetContainerResourcePolicy(containerName, podPolicy)
	if containerPolicy == nil || containerPolicy.ControlledMode == nil {
		return mpaTypes.ContainerControlledRequestsAndLimits
	}
	return *containerPolicy.ControlledMode
}

// GetControllingMpaForPod 获取管理指定pod的mpa(with labelSelector)
func GetControllingMpaForPod(pod *corev1.Pod, mpas []*MpaWithSelector) *MpaWithSelector {
	var controlling = &MpaWithSelector{
		Mpa:      nil,
		Selector: nil,
	}
	for _, mpa := range mpas {
		if PodMatchesMpa(pod, mpa) && strongerMpa(mpa.Mpa, controlling.Mpa) {
			controlling = mpa
		}
	}
	return controlling
}

// UpdateMpaStatusIfNeeded 根据新旧状态是否一致来确定是否需要更新MPA Object的状态(状态变化时更新，即不一致 -> 一致)
func UpdateMpaStatusIfNeeded(mpaClient clientType.MultidimPodAutoscalerInterface, mpaName string,
	oldStatus, newStatus *mpaTypes.MultidimPodAutoscalerStatus) (*mpaTypes.MultidimPodAutoscaler, error) {
	patches := []patch.Patch{
		{
			Op:    patch.Add,
			Path:  "/status",
			Value: *newStatus,
		},
	}

	if !equality.Semantic.DeepEqual(*oldStatus, *newStatus) {
		return patchMpa(mpaClient, mpaName, patches)
	}
	return nil, nil
}

// PodMatchesMpa 判断指定 pod 是否由 给定 mpa 管理
func PodMatchesMpa(pod *corev1.Pod, mpa *MpaWithSelector) bool {
	return PodLabelsMatchMpa(pod.Namespace, pod.GetLabels(), mpa.Mpa.Namespace, mpa.Selector)
}

// PodLabelsMatchMpa 判断pod的label是否可以匹配到给定的mpa selector
func PodLabelsMatchMpa(podNamespece string, labels labels.Set, mpaNamespace string, mpaSelector labels.Selector) bool {
	if podNamespece != mpaNamespace {
		// 命名空间直接不匹配
		return false
	}
	// 通过 label selector 匹配
	return mpaSelector.Matches(labels)
}

// strongerMpa 判断管理一个相同的pod的两个MPA对象的优先级
// 1. 创建时间早的优先
// 2. name 字母序靠前的优先
func strongerMpa(a, b *mpaTypes.MultidimPodAutoscaler) bool {
	if b == nil {
		return true
	}
	// 比较创建时间
	var aTime, bTime metav1.Time
	aTime = a.GetCreationTimestamp()
	bTime = b.GetCreationTimestamp()
	if !aTime.Equal(&bTime) {
		return aTime.Before(&bTime)
	}
	// If the timestamps are the same (unlikely, but possible e.g. in test environments): compare by name to have a complete deterministic order.
	return a.GetName() < b.GetName()
}

// patchMpa 将指定的patch操作应用到给定的mpa上
func patchMpa(mpaClient clientType.MultidimPodAutoscalerInterface, mpaName string, patches []patch.Patch) (*mpaTypes.MultidimPodAutoscaler, error) {
	bytes, err := json.Marshal(patches)
	if err != nil {
		klog.Errorf("Cannot marshal MPA status patches %+v. Reason: %+v", patches, err)
		return nil, nil
	}
	return mpaClient.Patch(context.TODO(), mpaName, types.JSONPatchType, bytes, metav1.PatchOptions{})
}
