package updater

import (
	"context"
	"flag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	kubeClient "k8s.io/client-go/kubernetes"
	cliFlag "k8s.io/component-base/cli/flag"
	"k8s.io/klog"
	mpaClientset "multidim-pod-autoscaler/pkg/client/clientset/versioned"
	"multidim-pod-autoscaler/pkg/target"
	"multidim-pod-autoscaler/pkg/updater/priority"
	updaterUtil "multidim-pod-autoscaler/pkg/updater/util"
	"multidim-pod-autoscaler/pkg/util"
	"multidim-pod-autoscaler/pkg/util/metrics"
	utilMpa "multidim-pod-autoscaler/pkg/util/mpa"
	"time"
)

const (
	// 默认的cache再同步时间间隔
	defaultResyncPeriod = 10 * time.Minute
)

var (
	updaterInterval = flag.Duration("updater-interval", 1*time.Minute,
		"updater的主流程运行频率")

	minReplicasToUpdate = flag.Int("min-replicas", 1,
		"执行update的最少的副本数量")

	evictionFraction = flag.Float64("eviction-fraction", 0.5,
		"可以驱逐的副本个数占预配置个数的比例")

	prometheusAddress = flag.String("address", ":8945", "Prometheus metrics对外暴露的地址")
	kubeconfig        = flag.String("kubeconfig", "", "Path to kubeconfig. 使用out-cluster配置时指定")
	kubeApiQps        = flag.Float64("kube-api-qps", 5.0, "访问API-Server的 QPS 限制")
	kubeApiBurst      = flag.Int("kube-api-burst", 10, "访问API-Server的 QPS 峰值限制")

	mpaObjectNamespace = flag.String("mpa-object-namespace", corev1.NamespaceAll, "搜索MPA Objects的命名空间")
)

func main() {
	klog.InitFlags(nil)
	cliFlag.InitFlags()
	klog.V(1).Infof("Multidim Pod Autoscaler %s Updater", utilMpa.MultidimPodAutoscalerVersion)

	metrics.InitializeMetrics(*prometheusAddress)
	updaterUtil.RegisterMetrics()

	config := util.CreateKubeConfig(*kubeconfig, float32(*kubeApiQps), *kubeApiBurst)
	kubeclient := kubeClient.NewForConfigOrDie(config)
	mpaClient := mpaClientset.NewForConfigOrDie(config)
	factory := informers.NewSharedInformerFactory(kubeclient, defaultResyncPeriod)
	targetSelectorFetcher := target.NewMpaTargetSelectorFetcher(config, kubeclient, factory)

	updater, err := NewUpdater(
		kubeclient,
		mpaClient,
		*minReplicasToUpdate,
		*evictionFraction,
		targetSelectorFetcher,
		priority.NewProcessor(),
		*mpaObjectNamespace,
	)
	if err != nil {
		klog.Fatalf("failed to create MPA updater: %v", err)
	}
	ticker := time.Tick(*updaterInterval)
	for range ticker {
		ctx, cancel := context.WithTimeout(context.Background(), *updaterInterval)
		defer cancel()
		updater.MainProcedure(ctx)
	}
}
