package updater

import (
	"context"
	"fmt"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubeClient "k8s.io/client-go/kubernetes"
	clientListers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	mpaTypes "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"
	mpaClientset "multidim-pod-autoscaler/pkg/client/clientset/versioned"
	mpaListers "multidim-pod-autoscaler/pkg/client/listers/autoscaling/v1"
	"multidim-pod-autoscaler/pkg/target"
	"multidim-pod-autoscaler/pkg/updater/eviction"
	"multidim-pod-autoscaler/pkg/updater/priority"
	updaterUtil "multidim-pod-autoscaler/pkg/updater/util"
	utilMpa "multidim-pod-autoscaler/pkg/util/mpa"
	utilPod "multidim-pod-autoscaler/pkg/util/pod"
)

// Updater 用于更新pod来应用recommender的推荐资源方案
type Updater interface {
	// MainProcedure 为updater的一个主流程
	// (updater为一个定时循环任务)
	MainProcedure(ctx context.Context)
}

// updater 实现 Updater 接口
type updater struct {
	kubeclientset             kubeClient.Interface
	mpaclientset              mpaClientset.Interface
	scaleNamespacer           scale.ScalesGetter
	mapper                    meta.RESTMapper
	mpaLister                 mpaListers.MultidimPodAutoscalerLister
	podLister                 clientListers.PodLister
	eventRecorder             record.EventRecorder
	evictorFactory            eviction.PodEvictorFactory
	mpaTargetSelectorFetcher  target.MpaTargetSelectorFetch
	evictionPriorityProcessor priority.Processor
}

func NewUpdater(
	kubeclient kubeClient.Interface, mpaClient mpaClientset.Interface,
	scaleNamespacer scale.ScalesGetter,
	mapper meta.RESTMapper,
	minReplicasToUpdate int, evictionFraction float64,
	mpaTargetSelectorFetcher target.MpaTargetSelectorFetch,
	evictionPriorityProcessor priority.Processor,
	namespace string,
) (Updater, error) {
	evictorFactory, err := eviction.NewPodEvictorFactory(kubeclient, minReplicasToUpdate, evictionFraction)
	if err != nil {
		return nil, fmt.Errorf("failed to create evictor factory: %v", err)
	}
	return &updater{
		kubeclientset:             kubeclient,
		mpaclientset:              mpaClient,
		scaleNamespacer:           scaleNamespacer,
		mapper:                    mapper,
		mpaLister:                 utilMpa.NewMpasLister(mpaClient, namespace, make(chan struct{})),
		podLister:                 utilPod.NewPodLister(kubeclient, namespace, make(chan struct{})),
		eventRecorder:             updaterUtil.NewEventRecorder(kubeclient, "mpa-updater"),
		evictorFactory:            evictorFactory,
		mpaTargetSelectorFetcher:  mpaTargetSelectorFetcher,
		evictionPriorityProcessor: evictionPriorityProcessor,
	}, nil
}

// MainProcedure 实现 Updater 接口，执行updater主流程
func (u *updater) MainProcedure(ctx context.Context) {
	executionTimer := updaterUtil.NewExecutionTimer()
	// 观测整个主流程的用时
	defer executionTimer.ObserveTotal()

	mpaList, err := u.mpaLister.List(labels.Everything())
	if err != nil {
		klog.Fatalf("faied to get MPA Object list: %v", err)
	}
	mpas := make([]*utilMpa.MpaWithSelector, 0)
	for _, mpa := range mpaList {
		updateMode := utilMpa.GetMpaUpdateMode(mpa)
		if updateMode != mpaTypes.UpdateModeAuto {
			klog.V(3).Infof("skipped MPA Object %v/%v(its update mode was set to off(default is Auto))", mpa.Namespace, mpa.Name)
			continue
		}
		condition := utilMpa.GetMpaLatestCondition(mpa)
		// 只有在 RecommendationProvided (推荐方案可用且可以更新到pods) 状态下才更新
		if condition.Type != mpaTypes.RecommendationProvided {
			klog.V(3).Infof("skipped MPA Object %v/%v(its latest condition was %v)", mpa.Namespace, mpa.Name, condition)
			continue
		}
		selector, err := u.mpaTargetSelectorFetcher.Fetch(mpa)
		if err != nil {
			klog.V(3).Infof("skipped MPA Object %v/%v(connot fetch the target reference selector for it)", mpa.Namespace, mpa.Name)
			continue
		}
		mpas = append(mpas,
			&utilMpa.MpaWithSelector{
				Mpa:      mpa,
				Selector: selector,
			},
		)
	}
	if len(mpas) <= 0 {
		klog.Warningf("no MPA Obejects to process the update")
	}

	executionTimer.ObserveStep("GetMPAs")

	podList, err := u.podLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("failed to get pods list: %v", err)
		return
	}

	executionTimer.ObserveStep("GetPods")

	livingPods := filterDeletedPods(podList)
	mpaControlledPods := make(map[*mpaTypes.MultidimPodAutoscaler][]*corev1.Pod)
	for _, pod := range livingPods {
		controllingMpa := utilMpa.GetControllingMpaForPod(pod, mpas)
		if controllingMpa != nil {
			mpaControlledPods[controllingMpa.Mpa] = append(mpaControlledPods[controllingMpa.Mpa], pod)
		}
	}

	executionTimer.ObserveStep("FilterPods")

	for mpa, pods := range mpaControlledPods {
		evictor := u.evictorFactory.NewPodEvictor(pods)
		podsUpdateOrder := u.evictionPriorityProcessor.GetPodsUpdateOrder(filterNonEvictablePods(pods, evictor), mpa)
		for _, pod := range podsUpdateOrder {
			// 同一个controller下的pod被驱逐后，可能影响到其他pod的可驱逐状态
			// 需要二次检查
			if !evictor.Evictable(pod) {
				continue
			}
			klog.V(2).Infof("evicting pod %s/%s", pod.Namespace, pod.Name)
			err := evictor.Evict(pod, u.eventRecorder)
			if err != nil {
				klog.Warningf("failed to evict pod %s/%s: %v", pod.Namespace, pod.Name, err)
			}
		}
		scaleObj, targetGroupResource, err := u.getScaleResource(mpa)
		if err != nil {
			klog.Warningf("failed to get targetRef scale resource of MPA %s/%s: %v", mpa.Namespace, mpa.Name, err)
		} else {
			err = u.updateScaleResourceReplicas(mpa, scaleObj, targetGroupResource)
			if err != nil {
				klog.Warningf("failed to update targetRef scale resource of MPA %s/%s: %v", mpa.Namespace, mpa.Name, err)
			}
		}
	}
	executionTimer.ObserveStep("EvictPods")
}

// filterNonEvictablePods 过滤不可驱逐的pods
func filterNonEvictablePods(pods []*corev1.Pod, evictor eviction.PodEvictor) []*corev1.Pod {
	result := make([]*corev1.Pod, 0)
	for _, pod := range pods {
		if evictor.Evictable(pod) {
			result = append(result, pod)
		}
	}
	return result
}

// filterDeletedPods 过滤已被删除的pods
func filterDeletedPods(pods []*corev1.Pod) []*corev1.Pod {
	result := make([]*corev1.Pod, 0)
	for _, pod := range pods {
		// 删除时间戳被设置的都是即将被删除的pods
		if pod.DeletionTimestamp == nil {
			result = append(result, pod)
		}
	}
	return result
}

// getScaleResource 获取mpa指向的target对应的scale resource 及其对应的 groupResource
func (u *updater) getScaleResource(mpa *mpaTypes.MultidimPodAutoscaler) (*autoscalingv1.Scale, schema.GroupResource, error) {
	targetGroupVersion, err := schema.ParseGroupVersion(mpa.Spec.TargetRef.APIVersion)
	if err != nil {
		u.eventRecorder.Event(mpa, corev1.EventTypeWarning, "FailedGetScale", err.Error())
		return nil,
			schema.GroupResource{},
			fmt.Errorf("invalid API version in scale target reference of MPA(%s/%s): %v", mpa.Namespace, mpa.Name, err)
	}

	targetGroupKind := schema.GroupKind{
		Group: targetGroupVersion.Group,
		Kind:  mpa.Spec.TargetRef.Kind,
	}

	mappings, err := u.mapper.RESTMappings(targetGroupKind)
	if err != nil {
		u.eventRecorder.Event(mpa, corev1.EventTypeWarning, "FailedGetScale", err.Error())
		return nil,
			schema.GroupResource{},
			fmt.Errorf("unable to determine resource for scale target reference of MPA(%s/%s): %v", mpa.Namespace, mpa.Name, err)
	}

	var (
		targetGroupResource schema.GroupResource
		scaleObj            *autoscalingv1.Scale
	)
	for _, mapping := range mappings {
		targetGroupResource = mapping.Resource.GroupResource()
		scaleObj, err =
			u.scaleNamespacer.Scales(mpa.Namespace).Get(context.TODO(), targetGroupResource, mpa.Spec.TargetRef.Name, metav1.GetOptions{})
		if err == nil {
			break
		}
	}

	if err != nil {
		u.eventRecorder.Event(mpa, corev1.EventTypeWarning, "FailedGetScale", err.Error())
		return nil,
			schema.GroupResource{},
			fmt.Errorf("failed to query scale subresource for scale target reference of MPA(%s/%s): %v", mpa.Namespace, mpa.Name, err)
	}

	return scaleObj, targetGroupResource, nil
}

func (u *updater) updateScaleResourceReplicas(
	mpa *mpaTypes.MultidimPodAutoscaler,
	scaleObj *autoscalingv1.Scale,
	targetGR schema.GroupResource,
) error {
	oldReplicas := scaleObj.Spec.Replicas
	newReplicas := int32(mpa.Status.RecommendationResources.TargetPodNum)

	scaleObj.Spec.Replicas = newReplicas

	_, err := u.scaleNamespacer.Scales(mpa.Namespace).Update(context.TODO(), targetGR, scaleObj, metav1.UpdateOptions{})

	if err != nil {
		u.eventRecorder.Eventf(mpa, corev1.EventTypeWarning, "FailedScale", "New size: %d; error: %v", newReplicas, err.Error())
		return fmt.Errorf("failed to scale MPA(%s/%s)'s targetRef %s: %v", mpa.Namespace, mpa.Name, mpa.Spec.TargetRef.Name, err)
	}
	u.eventRecorder.Eventf(mpa, corev1.EventTypeNormal, "SuccessfulScale", "New size: %d;", newReplicas)
	klog.Infof("Successful rescale of %s, old size: %d, new size: %d, reason: %s",
		mpa.Name, oldReplicas, newReplicas)
	return nil
}
