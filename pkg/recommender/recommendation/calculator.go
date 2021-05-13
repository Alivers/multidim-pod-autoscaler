package recommendation

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
	"math"
	mpaTypes "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"
	"multidim-pod-autoscaler/pkg/recommender/metrics"
	"multidim-pod-autoscaler/pkg/recommender/util"
	utilMpa "multidim-pod-autoscaler/pkg/util/mpa"
)

var (
	cpuRequestMap = map[int64]int64{
		250:  6,
		500:  12,
		750:  20,
		1000: 26,
		1250: 34,
		1500: 40,
		1750: 46,
		2000: 52,
		2250: 60,
	}
	servicePenaltyCostMap = map[float64]float64{
		95.0: 1.0,
		90.0: 0.9,
		85.0: 0.8,
		80.0: 0.5,
		60.0: 0.3,
		0.0:  0.0,
	}
	// fractional constant
	factConst = []float64{1,
		1, 2, 6, 24, 120,
		720, 5040, 40320, 362880, 3628800,
		39916800, 479001600, 6227020800, 87178291200, 1307674368000,
		20922789888000,
	}
	factInverseConst = []float64{1,
		1, 0.5, 0.16666666666666666, 0.041666666666666664, 0.008333333333333333,
		0.001388888888888889, 0.0001984126984126984, 2.48015873015873e-05, 2.7557319223985893e-06, 2.755731922398589e-07,
		2.505210838544172e-08, 2.08767569878681e-09, 1.6059043836821613e-10, 1.1470745597729725e-11, 7.647163731819816e-13,
		4.779477332387385e-14,
	}
)

const (
	podNumMin int64 = 1
	podNumMax int64 = 16
	// defaultResponseTime 请求默认响应时间 ms
	defaultResponseTime = 300
	// cpuPrice cpu单价 vCore/s
	cpuPrice = 0.00003334
	// podCreateTime pod 创建时间 ms
	podCreateTime int64 = 5000
	// recommenderInterval 两次推荐的间隔时间
	recommenderInterval = int64(1 * 60 * 1000)
	// 资源成本与违约成本分别的占比
	resourceCostRatio = 0.5
	penaltyCostRatio  = 1.0 - resourceCostRatio
	// 资源成本的最值
	resourceCostMax = float64(podNumMax*2250) * cpuPrice
	resourceCostMin = float64(podNumMin*250) * cpuPrice
	// 推荐方案更新的阈值
	recommendationBetterThresold = 0.02
)

type RecommendationAction string

const (
	SkipRecommendation    RecommendationAction = "skipRecommendation"
	ApplyRecommendation   RecommendationAction = "applyRecommendation"
	UnknownRecommendation RecommendationAction = "unknownRecommendation"
)

type Calculator interface {
	Calculate(
		mpaWithSelector *utilMpa.MpaWithSelector,
		controlledPod []*corev1.Pod,
	) (*mpaTypes.RecommendedResources, RecommendationAction, error)
}

type calculator struct {
	metricsClient metrics.Client
}

func NewCalculator(client metrics.Client) Calculator {
	return &calculator{
		metricsClient: client,
	}
}

func (c *calculator) Calculate(
	mpaWithSelector *utilMpa.MpaWithSelector,
	controlledPod []*corev1.Pod,
) (*mpaTypes.RecommendedResources, RecommendationAction, error) {
	klog.V(2).Infof("attempt to get qps in Namespace(%s) with podSelector(%s)", mpaWithSelector.Mpa.Namespace, mpaWithSelector.Selector.String())
	// 获取pods的qps
	podsMetricsInfo, _, err :=
		c.metricsClient.GetPodRawMetric("http_requests", mpaWithSelector.Mpa.Namespace, mpaWithSelector.Selector, labels.NewSelector())
	if err != nil {
		return nil, UnknownRecommendation, fmt.Errorf("failed to get pods' qps: %v", err.Error())
	}

	klog.V(2).Infof("get qps metrics of pods: %v", podsMetricsInfo)

	var resourceFormat resource.Format

	// 统计所有 pod副本的 qps
	var serviceQps float64
	for _, pod := range controlledPod {
		metricsInfo, exists := podsMetricsInfo[util.GetPodId(pod)]
		if !exists {
			klog.Infof("connot get the http_requests metrics of pod(%s/%s)", pod.Namespace, pod.Name)
			continue
		}
		serviceQps += float64(metricsInfo.Value.MilliValue())
		resourceFormat = metricsInfo.Value.Format
	}
	// 将 1000m 为单元的转为 1.0 为单元的数值
	serviceQps = serviceQps / 1000.0
	// 请求的期望响应时间
	expectResponseTime := defaultResponseTime
	if mpaWithSelector.Mpa.Spec.ResourcePolicy != nil &&
		len(mpaWithSelector.Mpa.Spec.ResourcePolicy.ContainerPolicies) > 0 {
		expectResponseTime = mpaWithSelector.Mpa.Spec.ResourcePolicy.ContainerPolicies[0].ExpRespTime
	}
	var oldScore float64
	// 获取当前负载下的推荐方案
	score, targetPodNum, targetPodResource := recommendResource(serviceQps, expectResponseTime)
	targetQuantity := resource.NewMilliQuantity(targetPodResource, resourceFormat)

	// 计算旧的资源方案在新的qps下的得分
	if mpaWithSelector.Mpa.Status.RecommendationResources != nil {
		var cpuQuantity resource.Quantity
		if len(mpaWithSelector.Mpa.Status.RecommendationResources.ContainerRecommendations) > 0 {
			cpuQuantity = mpaWithSelector.Mpa.Status.RecommendationResources.ContainerRecommendations[0].Target[corev1.ResourceCPU]
		}
		podNum := mpaWithSelector.Mpa.Status.RecommendationResources.TargetPodNum
		reqs := cpuRequestMap[cpuQuantity.MilliValue()]
		oldScore = evaluatePolicy(
			cpuQuantity.MilliValue(),
			int64(podNum),
			reqs,
			serviceQps,
			float64(expectResponseTime),
			serviceQps/float64(int64(podNum)*reqs),
		)
	}

	// 如果旧方案得分为零(无方案) 或 当前方案得分超出旧方案得分 threshold 则进行更新
	if oldScore < 0.0000001 || (score-oldScore)/oldScore > recommendationBetterThresold {
		return &mpaTypes.RecommendedResources{
			TargetPodNum: int(targetPodNum),
			ContainerRecommendations: []mpaTypes.RecommendedContainerResources{
				{
					Target:        corev1.ResourceList{corev1.ResourceCPU: *targetQuantity},
					ContainerName: mpaTypes.DefaultContainerResourcePolicy,
				},
			},
		}, ApplyRecommendation, nil
	}

	return &mpaTypes.RecommendedResources{}, SkipRecommendation, nil
}

// recommendResource 通过伸缩推荐算法计算资源方案
func recommendResource(qps float64, expectRespTime int) (float64, int64, int64) {
	var curPodNum, curCpuQuantity int64
	var curScore float64

	for cpu, reqs := range cpuRequestMap {
		waitTime := float64(expectRespTime) - 1.0/float64(reqs)
		for podNum := podNumMin; podNum <= podNumMax; podNum += 1 {
			// 服务强度 ρ
			serviceIntensity := qps / float64(podNum*reqs)

			score := evaluatePolicy(cpu, podNum, reqs, qps, waitTime, serviceIntensity)
			// 更新推荐方案
			if score > curScore {
				curScore = score
				curPodNum = podNum
				curCpuQuantity = cpu
			}
		}
	}

	klog.V(4).Infof("final policy(score=%g): instance number=%d, instance resources=%dm", curScore, curPodNum, curCpuQuantity)

	return curScore, curPodNum, curCpuQuantity
}

// evaluatePolicy 计算给定资源方案的得分
func evaluatePolicy(res, podNum, reqs int64, qps float64, waitTime, serviceIntensity float64) float64 {
	// 如果出现无限排队 跳过
	if serviceIntensity >= 1.0 {
		klog.V(2).Infof("policy(cpuQuantity=%dm,podNum=%d,qps=%g,req/s=%d) maybe lead to infinite queueing, skipped this policy", res, podNum, qps, reqs)
		return 0.0
	}

	serviceScore := queueRequests(reqs, podNum, qps, waitTime, serviceIntensity)
	// 通过资源成本和违约成本计算方案得分
	resCost := calculateResourceCost(res, podNum)
	penaltyCost := calculatePenaltyCost(serviceScore)
	score := calculatePolicyScore(resCost, penaltyCost)

	klog.V(4).Infof("policy (cpuQuantity=%dm,podNum=%d,req/s=%d,qps=%g,serviceIntensity=%g,waitTime=%gms) with score(serviceScore=%g,resourceCost=%g,penaltyCost=%g,finalScore=%g)", res, podNum, reqs, qps, serviceIntensity, waitTime, serviceScore, resCost, penaltyCost, score)

	return score
}

func calculateResourceCost(res int64, podNum int64) float64 {
	// 资源量 * 单价
	cost := float64(podNum*res) * cpuPrice
	// 最大最小归一化
	return (resourceCostMax - cost) / (resourceCostMax - resourceCostMin)
}

// calculatePenaltyCost 计算违约成本
func calculatePenaltyCost(serviceScore float64) float64 {
	for score, Cost := range servicePenaltyCostMap {
		if serviceScore >= score {
			return Cost
		}
	}
	return 1.0
}

// calculatePolicyScore 计算方案得分(归一化两个成本并乘以各自的权重)
func calculatePolicyScore(resourceCost, penaltyCost float64) float64 {
	return resourceCost*resourceCostRatio + penaltyCost*penaltyCostRatio
}

// queueRequests 通过 M/M/C 排队论模型，评估输入方案的服务可用性
func queueRequests(reqs, podNum int64, qps, waitTime float64, serviceIntensity float64) float64 {
	// λ / μ
	tmp := qps / float64(reqs)
	p0 := func() float64 {
		sum := 0.0
		for i := 0; i < int(podNum); i += 1 {
			sum += factInverseConst[i] * math.Pow(tmp, float64(i))
		}
		sum += factInverseConst[podNum] * (1.0 / (1.0 - serviceIntensity)) * math.Pow(tmp, float64(podNum))
		return 1.0 / sum
	}()

	lengthQueue :=
		math.Pow(float64(podNum)*serviceIntensity, float64(podNum)) * serviceIntensity / (factConst[podNum] * (1 - serviceIntensity) * (1 - serviceIntensity)) * p0

	serviceScore := (1 - math.Exp((waitTime/1000.0)*(qps-float64(podNum*reqs)))) * (100 * lengthQueue)
	return serviceScore
}
