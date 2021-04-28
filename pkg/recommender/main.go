package recommender

import (
	"context"
	"flag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	kubeClient "k8s.io/client-go/kubernetes"
	cliFlag "k8s.io/component-base/cli/flag"
	"k8s.io/klog"
	mpaClientset "multidim-pod-autoscaler/pkg/client/clientset/versioned"
	recommenderMetrics "multidim-pod-autoscaler/pkg/recommender/metrics"
	"multidim-pod-autoscaler/pkg/recommender/recommendation"
	recommenderUtil "multidim-pod-autoscaler/pkg/recommender/util"
	"multidim-pod-autoscaler/pkg/target"
	updaterUtil "multidim-pod-autoscaler/pkg/updater/util"
	"multidim-pod-autoscaler/pkg/util"
	"multidim-pod-autoscaler/pkg/util/limitrange"
	"multidim-pod-autoscaler/pkg/util/metrics"
	utilMpa "multidim-pod-autoscaler/pkg/util/mpa"
	utilRecommendation "multidim-pod-autoscaler/pkg/util/recommendation"
	"time"
)

const (
	// 默认的cache再同步时间间隔
	defaultResyncPeriod = 10 * time.Minute
)

var (
	recommenderInterval = flag.Duration("recommender-interval", 1*time.Minute,
		"recommender的主流程运行频率")

	prometheusAddress = flag.String("address", ":8946", "Prometheus metrics对外暴露的地址")
	kubeconfig        = flag.String("kubeconfig", "", "Path to kubeconfig. 使用out-cluster配置时指定")
	kubeApiQps        = flag.Float64("kube-api-qps", 5.0, "访问API-Server的 QPS 限制")
	kubeApiBurst      = flag.Int("kube-api-burst", 10, "访问API-Server的 QPS 峰值限制")

	mpaObjectNamespace = flag.String("mpa-object-namespace", corev1.NamespaceAll, "搜索MPA Objects的命名空间")
)

func main() {
	klog.InitFlags(nil)
	cliFlag.InitFlags()
	klog.V(1).Infof("Multidim Pod Autoscaler %s Recommender", utilMpa.MultidimPodAutoscalerVersion)

	metrics.InitializeMetrics(*prometheusAddress)
	updaterUtil.RegisterMetrics()

	config := util.CreateKubeConfig(*kubeconfig, float32(*kubeApiQps), *kubeApiBurst)
	kubeclient := kubeClient.NewForConfigOrDie(config)
	mpaClient := mpaClientset.NewForConfigOrDie(config)
	factory := informers.NewSharedInformerFactory(kubeclient, defaultResyncPeriod)
	targetSelectorFetcher := target.NewMpaTargetSelectorFetcher(config, kubeclient, factory)

	customMetricsClient := recommenderUtil.NewCustomMetricsClient(config)
	recommendationCalculator := recommendation.NewCalculator(recommenderMetrics.NewClient(customMetricsClient))

	limitRangeCalculator, err := limitrange.NewCalculator(factory)
	if err != nil {
		limitRangeCalculator = nil
	}
	recommendationProcessor := utilRecommendation.NewProcessor(limitRangeCalculator)

	recommender, err := NewRecommender(
		kubeclient,
		mpaClient,
		targetSelectorFetcher,
		recommendationCalculator,
		recommendationProcessor,
		*mpaObjectNamespace,
	)
	if err != nil {
		klog.Fatalf("failed to create MPA updater: %v", err)
	}
	ticker := time.Tick(*recommenderInterval)
	for range ticker {
		ctx, cancel := context.WithTimeout(context.Background(), *recommenderInterval)
		defer cancel()
		recommender.MainProcedure(ctx)
	}
}
