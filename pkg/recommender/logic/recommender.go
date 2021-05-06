package logic

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubeClient "k8s.io/client-go/kubernetes"
	coreListers "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog"
	mpaTypes "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"
	mpaClientset "multidim-pod-autoscaler/pkg/client/clientset/versioned"
	mpaListers "multidim-pod-autoscaler/pkg/client/listers/autoscaling/v1"
	"multidim-pod-autoscaler/pkg/recommender/recommendation"
	"multidim-pod-autoscaler/pkg/target"
	utilMpa "multidim-pod-autoscaler/pkg/util/mpa"
	utilPod "multidim-pod-autoscaler/pkg/util/pod"
	recommendationUtil "multidim-pod-autoscaler/pkg/util/recommendation"
)

type Recommender interface {
	MainProcedure(ctx context.Context)
}

type recommender struct {
	kubeclientset            kubeClient.Interface
	mpaclientset             mpaClientset.Interface
	mpaLister                mpaListers.MultidimPodAutoscalerLister
	podLister                coreListers.PodLister
	mpaTargetSelectorFetcher target.MpaTargetSelectorFetch
	recommendationCalculator recommendation.Calculator
	recommendationProcessor  recommendationUtil.Processor
}

func NewRecommender(
	kubeclient kubeClient.Interface,
	mpaclient mpaClientset.Interface,
	mpaTargetSelectorFetcher target.MpaTargetSelectorFetch,
	recommendationCalculator recommendation.Calculator,
	recommendationProcessor recommendationUtil.Processor,
	namespace string) (Recommender, error) {
	return &recommender{
		kubeclientset:            kubeclient,
		mpaclientset:             mpaclient,
		mpaLister:                utilMpa.NewMpasLister(mpaclient, namespace, make(chan struct{})),
		podLister:                utilPod.NewPodLister(kubeclient, namespace, make(chan struct{})),
		mpaTargetSelectorFetcher: mpaTargetSelectorFetcher,
		recommendationProcessor:  recommendationProcessor,
		recommendationCalculator: recommendationCalculator,
	}, nil
}

func (r *recommender) MainProcedure(ctx context.Context) {
	// 获取 mpas
	mpaList, err := r.mpaLister.List(labels.Everything())
	if err != nil {
		klog.Fatalf("faied to get MPA Object list: %v", err)
	}
	// 获取每个mpa对应的pod label selector
	mpas := make([]*utilMpa.MpaWithSelector, 0)
	for _, mpa := range mpaList {
		updateMode := utilMpa.GetMpaUpdateMode(mpa)
		if updateMode != mpaTypes.UpdateModeAuto {
			klog.V(3).Infof("skipped MPA Object %v/%v(its update mode was set to off(default is Auto))", mpa.Namespace, mpa.Name)
			continue
		}
		selector, err := r.mpaTargetSelectorFetcher.Fetch(mpa)
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
	// 获取pod列表
	podList, err := r.podLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("failed to get pods list: %v", err)
		return
	}
	// 过滤杯驱逐(待删除)的pod
	livingPods := filterDeletedPods(podList)
	// 匹配每个mpa控制的pods
	mpaControlledPods := make(map[*utilMpa.MpaWithSelector][]*corev1.Pod)
	for _, pod := range livingPods {
		controllingMpa := utilMpa.GetControllingMpaForPod(pod, mpas)
		if controllingMpa != nil {
			mpaControlledPods[controllingMpa] = append(mpaControlledPods[controllingMpa], pod)
		}
	}

	for mpaWithSelector, pods := range mpaControlledPods {
		if len(pods) <= 0 {
			klog.Infof("MPA(%s/%s) has not controlled any pods", mpaWithSelector.Mpa.Namespace, mpaWithSelector.Mpa.Name)
			continue
		}
		// 计算推荐方案
		recommendationRes, action := r.recommendationCalculator.Calculate(mpaWithSelector, pods)

		klog.V(4).Infof("calculate recommendation finished(action: %s, value: %v)", action, *recommendationRes)

		var adjustRecommendation *mpaTypes.RecommendedResources
		var newCondition mpaTypes.MultidimPodAutoscalerCondition
		if action == recommendation.ApplyRecommendation {
			// 调整推荐方案
			adjustRecommendation, _, err =
				r.recommendationProcessor.AdjustRecommendation(recommendationRes, mpaWithSelector.Mpa.Spec.ResourcePolicy, pods[0])
			if err != nil {
				klog.Errorf("failed to adjust the recommendation resources of MPA(%s/%s): %v", mpaWithSelector.Mpa.Namespace, mpaWithSelector.Mpa.Name, err)
				continue
			}
			newCondition.Status = corev1.ConditionTrue
			newCondition.LastTransitionTime = metav1.Now()
			newCondition.Type = mpaTypes.RecommendationProvided
			newCondition.Reason = "Recommendation Provided"
		} else if action == recommendation.SkipRecommendation {
			newCondition.Status = corev1.ConditionTrue
			newCondition.LastTransitionTime = metav1.Now()
			newCondition.Type = mpaTypes.RecommendationSkipped
			newCondition.Reason = "Recommendation Skipped"
		} else {
			newCondition.Status = corev1.ConditionTrue
			newCondition.LastTransitionTime = metav1.Now()
			newCondition.Type = mpaTypes.RecommendationSkipped
			newCondition.Reason = "Recommendation Unknown"
		}

		// 如果必要，更新推荐方案
		_, err = r.updateRecommendationIfBetter(adjustRecommendation, newCondition, mpaWithSelector.Mpa)
		if err != nil {
			klog.Errorf("failed to update the recommendation resources for MPA(%s/%s): %v", mpaWithSelector.Mpa.Namespace, mpaWithSelector.Mpa.Name, err)
		} else {
			klog.V(4).Infof("Successful recommendation for MPA(%s/%s): %v", mpaWithSelector.Mpa.Namespace, mpaWithSelector.Mpa.Name, *adjustRecommendation)
		}
	}
}

// updateRecommendationIfBetter 更新mpa对象的资源推荐方案
func (r *recommender) updateRecommendationIfBetter(
	newRecommendation *mpaTypes.RecommendedResources,
	newStatusCondition mpaTypes.MultidimPodAutoscalerCondition,
	mpa *mpaTypes.MultidimPodAutoscaler) (bool, error) {
	mpaCopy := mpa.DeepCopy()
	if newRecommendation != nil {
		mpaCopy.Status.RecommendationResources = newRecommendation.DeepCopy()
	}
	mpaCopy.Status.Conditions = append(mpaCopy.Status.Conditions, newStatusCondition)

	_, err :=
		r.mpaclientset.AutoscalingV1().MultidimPodAutoscalers(mpa.Namespace).Update(context.TODO(), mpaCopy, metav1.UpdateOptions{})
	return true, err
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
