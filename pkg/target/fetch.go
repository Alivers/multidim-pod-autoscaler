package target

import (
	"context"
	"fmt"
	appsV1 "k8s.io/api/apps/v1"
	batchV1 "k8s.io/api/batch/v1"
	batchV1Beta1 "k8s.io/api/batch/v1beta1"
	coreV1 "k8s.io/api/core/v1"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	cachedDiscovery "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	kubeClient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	mpaTypes "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"
	"time"
)

const (
	// discovery client 缓存重置的时间间隔
	discoveryResetPeriod = 5 * time.Minute
)

// MpaTargetSelectorFetch 获取 labelSelector，用于选择被指定 MPA 控制的 PODs
type MpaTargetSelectorFetch interface {
	// Fetch 如果返回 error == nil, 则 selector 不是 nil
	Fetch(mpa *mpaTypes.MultidimPodAutoscaler) (labels.Selector, error)
}

// WellKnownController 常见控制器枚举 枚举常量类型定义
type WellKnownController string

// 常见控制器枚举
const (
	DaemonSet             WellKnownController = "DaemonSet"
	Deployment            WellKnownController = "Deployment"
	ReplicaSet            WellKnownController = "ReplicaSet"
	StatefulSet           WellKnownController = "StatefulSet"
	ReplicationController WellKnownController = "ReplicationController"
	Job                   WellKnownController = "Job"
	CronJob               WellKnownController = "CronJob"
)

// NewMpaTargetSelectorFetcher 返回 MpaTargetSelectorFetcher 接口，来获指定 mpa 的label选择器
// config - client配置；kubeClient - client；factory - 用于创建informer对象
// 使用 sharedInformer (一个mpa上可能包含多个controller，每个控制器注册自己的回调，共享store)
func NewMpaTargetSelectorFetcher(config *rest.Config, kubeClient kubeClient.Interface, factory informers.SharedInformerFactory) MpaTargetSelectorFetch {
	// 用于获取 api-server 支持的资源组、版本、信息
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		klog.Fatalf("Cannot create discovery client: %v", err)
	}

	// scale 子资源中资源和 (组, 版本, kind) 之间的对应关系
	resolver := scale.NewDiscoveryScaleKindResolver(discoveryClient)
	// base client 用于创建 scale 子资源
	restClient := kubeClient.CoreV1().RESTClient()
	// 带缓存的discoveryClient
	cachedDiscoveryClient := cachedDiscovery.NewMemCacheClient(discoveryClient)
	// 懒加载 rest mapper(将resource映射到kind)
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedDiscoveryClient)

	// channel 必须使用 make 构造
	go wait.Until(func() {
		// 重置缓存信息(会发出mapping请求，初始化/更新REST mapper)
		mapper.Reset()
	}, discoveryResetPeriod, make(chan struct{}))

	// 构造 informers map, informer 使用 factory 工厂创建
	informersMap := map[WellKnownController]cache.SharedIndexInformer{
		DaemonSet:             factory.Apps().V1().DaemonSets().Informer(),
		Deployment:            factory.Apps().V1().Deployments().Informer(),
		ReplicaSet:            factory.Apps().V1().ReplicaSets().Informer(),
		StatefulSet:           factory.Apps().V1().StatefulSets().Informer(),
		ReplicationController: factory.Core().V1().ReplicationControllers().Informer(),
		Job:                   factory.Batch().V1().Jobs().Informer(),
		CronJob:               factory.Batch().V1beta1().CronJobs().Informer(),
	}

	for kind, informer := range informersMap {
		// 启动informer
		stopChan := make(chan struct{})
		go informer.Run(stopChan)
		// 等待informer的cache被填充(只可能返回true/false)
		// stopChan在这里不会被主动关闭
		synced := cache.WaitForCacheSync(stopChan, informer.HasSynced)
		if !synced {
			klog.Fatalf("cannot sync cache for %s: %v", kind, err)
		} else {
			klog.Infof("initial sync of %s completed", kind)
		}
	}
	// 创建scale子资源接口(用于获取或更新某个namespace下定义了的scale子资源)
	scaleNamespacer := scale.New(restClient, mapper, dynamic.LegacyAPIPathResolverFunc, resolver)
	return &mpaTargetSelectorFetcher{
		scaleNamespacer: scaleNamespacer,
		mapper:          mapper,
		informersMap:    informersMap,
	}
}

// mpaTargetSelectorFetcher 实现 MpaTargetSelectorFetcher 接口
// 通过 API server 查询 mpa 指向的 controller
type mpaTargetSelectorFetcher struct {
	scaleNamespacer scale.ScalesGetter
	mapper          apiMeta.RESTMapper
	informersMap    map[WellKnownController]cache.SharedIndexInformer
}

// Fetch 实现 MpaTargetSelectorFetcher 接口
func (fetch *mpaTargetSelectorFetcher) Fetch(mpa *mpaTypes.MultidimPodAutoscaler) (labels.Selector, error) {
	if mpa.Spec.TargetRef == nil {
		return nil, fmt.Errorf("targetRef undefined")
	}

	kind := WellKnownController(mpa.Spec.TargetRef.Kind)

	informer, existed := fetch.informersMap[kind]

	// 当前 map 中可以找到
	if existed {
		return getLabelSelector(informer, mpa.Spec.TargetRef.Kind, mpa.Namespace, mpa.Spec.TargetRef.Name)
	}

	// 是未知的 controller(上面的枚举定义)
	// 使用 scale 子资源查询
	groupVersion, err := schema.ParseGroupVersion(mpa.Spec.TargetRef.APIVersion)
	if err != nil {
		return nil, err
	}
	groupKind := schema.GroupKind{
		Group: groupVersion.Group,
		Kind:  mpa.Spec.TargetRef.Kind,
	}

	selector, err := fetch.getLabelSelectorFromResource(groupKind, mpa.Namespace, mpa.Spec.TargetRef.Name)
	if err != nil {
		return nil, fmt.Errorf("unhandled targetRef %s / %s / %s, last error %v",
			mpa.Spec.TargetRef.APIVersion, mpa.Spec.TargetRef.Kind, mpa.Spec.TargetRef.Name, err)
	}
	return selector, nil
}

// getLabelSelector 获取 labelSelector
// 使用指定 informer 获取给定 namespace 下名为 name 的对象
// namespace 为 mpa 所在的空间, name 为该空间下的对象名
func getLabelSelector(informer cache.SharedIndexInformer, kind, namespace, name string) (labels.Selector, error) {
	// 通过 informer 的本地缓存获取对应的资源对象
	item, exists, err := informer.GetStore().GetByKey(namespace + "/" + name)

	// 返回错误
	if err != nil {
		return nil, err
	}

	// 不存在
	if !exists {
		return nil, fmt.Errorf("%s %s/%s does not exist", kind, namespace, name)
	}

	// 类型断言
	switch item.(type) {
	case *appsV1.DaemonSet:
		apiObj, ok := item.(*appsV1.DaemonSet)
		if !ok {
			return nil, fmt.Errorf("%s %s/%s failed to parse", kind, namespace, name)
		}
		return metaV1.LabelSelectorAsSelector(apiObj.Spec.Selector)
	case *appsV1.Deployment:
		apiObj, ok := item.(*appsV1.Deployment)
		if !ok {
			return nil, fmt.Errorf("%s %s/%s failed to parse", kind, namespace, name)
		}
		return metaV1.LabelSelectorAsSelector(apiObj.Spec.Selector)
	case *appsV1.ReplicaSet:
		apiObj, ok := item.(*appsV1.ReplicaSet)
		if !ok {
			return nil, fmt.Errorf("%s %s/%s failed to parse", kind, namespace, name)
		}
		return metaV1.LabelSelectorAsSelector(apiObj.Spec.Selector)
	case *appsV1.StatefulSet:
		apiObj, ok := item.(*appsV1.StatefulSet)
		if !ok {
			return nil, fmt.Errorf("%s %s/%s failed to parse", kind, namespace, name)
		}
		return metaV1.LabelSelectorAsSelector(apiObj.Spec.Selector)
	case *coreV1.ReplicationController:
		apiObj, ok := item.(*coreV1.ReplicationController)
		if !ok {
			return nil, fmt.Errorf("%s %s/%s failed to parse", kind, namespace, name)
		}
		// 这里的 selector 是 set，需要转换
		return metaV1.LabelSelectorAsSelector(metaV1.SetAsLabelSelector(apiObj.Spec.Selector))
	case *batchV1.Job:
		apiObj, ok := item.(*batchV1.Job)
		if !ok {
			return nil, fmt.Errorf("%s %s/%s failed to parse", kind, namespace, name)
		}
		return metaV1.LabelSelectorAsSelector(apiObj.Spec.Selector)
	case *batchV1Beta1.CronJob:
		apiObj, ok := item.(*batchV1Beta1.CronJob)
		if !ok {
			return nil, fmt.Errorf("%s %s/%s failed to parse", kind, namespace, name)
		}
		return metaV1.LabelSelectorAsSelector(metaV1.SetAsLabelSelector(apiObj.Spec.JobTemplate.Spec.Template.Labels))
	}
	return nil, fmt.Errorf("cannot find %s %s/%s", kind, namespace, name)
}

// getLabelSelectorFromResource 根据 groupKind 获取 namespace/name 对应的labelSelector
// REST mapping 实现了 groupKind 到资源的映射
func (fetch *mpaTargetSelectorFetcher) getLabelSelectorFromResource(groupKind schema.GroupKind, namespace, name string) (labels.Selector, error) {
	mappings, err := fetch.mapper.RESTMappings(groupKind)
	if err != nil {
		return nil, err
	}

	var retError error

	for _, mapping := range mappings {
		groupResource := mapping.Resource.GroupResource()
		// 获取 scale 子资源(实际创建 mpa 资源对象时绑定)
		subScale, err := fetch.scaleNamespacer.Scales(namespace).Get(context.TODO(), groupResource, name, metaV1.GetOptions{})

		if err == nil {
			// scale 子资源中的 selector
			if subScale.Status.Selector == "" {
				return nil, fmt.Errorf("resource %s/%s has an empty selector for scale sub-resource", namespace, name)
			}
			selector, err := labels.Parse(subScale.Status.Selector)
			if err != nil {
				return nil, err
			}
			return selector, nil
		}
		retError = err
	}

	return nil, retError
}
