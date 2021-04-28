package recommender

import (
	"context"
	corev1 "k8s.io/api/core/v1"
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
	mpaLister                mpaListers.MultidimPodAutoscalerLister
	podLister                coreListers.PodLister
	mpaTargetSelectorFetcher target.MpaTargetSelectorFetch
	recommendationCalculator recommendation.Calculator
	recommendationProcessor  recommendationUtil.Processor
}

func NewRecommender(
	kubeclient kubeClient.Interface,
	mpaclient *mpaClientset.Clientset,
	mpaTargetSelectorFetcher target.MpaTargetSelectorFetch,
	recommendationCalculator recommendation.Calculator,
	recommendationProcessor recommendationUtil.Processor,
	namespace string) (Recommender, error) {
	return &recommender{
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
	mpaControlledPods := make(map[*mpaTypes.MultidimPodAutoscaler][]*corev1.Pod)
	for _, pod := range livingPods {
		controllingMpa := utilMpa.GetControllingMpaForPod(pod, mpas)
		if controllingMpa != nil {
			mpaControlledPods[controllingMpa.Mpa] = append(mpaControlledPods[controllingMpa.Mpa], pod)
		}
	}

	for mpa, pods := range mpaControlledPods {
		if len(pods) <= 0 {
			klog.Infof("MPA(%s/%s) has not controlled any pods", mpa.Namespace, mpa.Name)
			continue
		}
		recommendationRes := r.recommendationCalculator.Calculate(mpa, pods)
		adjustRecommendation, _, err :=
			r.recommendationProcessor.AdjustRecommendation(recommendationRes, mpa.Spec.ResourcePolicy, pods[0])
		if err != nil {
			klog.Errorf("failed to adjust the recommendation resources of MPA(%s/%s): err", mpa.Namespace, mpa.Name)
			continue
		}
		_, err = updateRecommendationIfBetter(adjustRecommendation, mpa.Status.RecommendationResources, mpa)
		if err != nil {
			klog.Errorf("failed to update the recommendation resources for MPA(%s/%s): err", mpa.Namespace, mpa.Name)
		}
	}
}

func updateRecommendationIfBetter(
	newRecommendation *mpaTypes.RecommendedResources,
	oldRecomendation *mpaTypes.RecommendedResources,
	mpa *mpaTypes.MultidimPodAutoscaler) (bool, error) {

	return true, nil
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
