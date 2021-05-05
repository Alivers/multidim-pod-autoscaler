package main

import (
	"flag"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	cliFlag "k8s.io/component-base/cli/flag"
	"k8s.io/klog"
	podPatch "multidim-pod-autoscaler/pkg/admission/pod/patch"
	admissionUtil "multidim-pod-autoscaler/pkg/admission/util"
	clientSet "multidim-pod-autoscaler/pkg/client/clientset/versioned"
	"multidim-pod-autoscaler/pkg/target"
	"multidim-pod-autoscaler/pkg/util"
	"multidim-pod-autoscaler/pkg/util/limitrange"
	metricsUtil "multidim-pod-autoscaler/pkg/util/metrics"
	mpaUtil "multidim-pod-autoscaler/pkg/util/mpa"
	"multidim-pod-autoscaler/pkg/util/recommendation"
	"net/http"
	"os"
	"time"
)

const (
	// 默认的cache再同步时间间隔
	defaultResyncPeriod = 10 * time.Minute
)

var (
	certsConfiguration = &certsConfig{
		clientCaFile:  flag.String("client-ca-file", "/etc/mpa-tls-certs/caCert.pem", "CA证书的路径"),
		tlsCertFile:   flag.String("tls-cert-file", "/etc/mpa-tls-certs/serverCert.pem", "server证书的路径"),
		tlsPrivateKey: flag.String("tls-private-key", "/etc/mpa-tls-certs/serverKey.pem", "server秘钥的路径"),
	}
	port               = flag.Int("port", 8000, "webhook server 监听的端口号")
	prometheusAddress  = flag.String("address", ":8944", "Prometheus metrics对外暴露的地址")
	kubeconfig         = flag.String("kubeconfig", "", "Path to kubeconfig. 使用out-cluster配置时指定")
	kubeApiQps         = flag.Float64("kube-api-qps", 5.0, "访问API-Server的 QPS 限制")
	kubeApiBurst       = flag.Int("kube-api-burst", 10, "访问API-Server的 QPS 峰值限制")
	namespace          = os.Getenv("NAMESPACE")
	serviceName        = flag.String("webhook-service", "mpa-webhook", "当不使用url注册webhook时，需要指定webhook的服务名")
	webhookTimeout     = flag.Int("webhook-timeout-seconds", 30, "API-Server等待webhook响应的超时时间")
	mpaObjectNamespace = flag.String("mpa-object-namespace", corev1.NamespaceAll, "搜索MPA Objects的命名空间")
)

func main() {
	klog.InitFlags(nil)
	cliFlag.InitFlags()

	klog.V(1).Info("Multidim Pod Autoscaler(%s) Admission Controller", mpaUtil.MultidimPodAutoscalerVersion)

	// 初始化 prometheus metrics
	metricsUtil.InitializeMetrics(*prometheusAddress)
	// 注册 admission controller用到的 metrics tools
	admissionUtil.RegisterMetrics()

	// 初始化 tls 证书配置
	certs := initCerts(*certsConfiguration)
	// 创建kubeconfig
	kubeconfig := util.CreateKubeConfig(*kubeconfig, float32(*kubeApiQps), *kubeApiBurst)

	// 创建 mpa lister(获取所有mpa对象)
	mpaClientset := clientSet.NewForConfigOrDie(kubeconfig)
	mpaLister := mpaUtil.NewMpasLister(mpaClientset, *mpaObjectNamespace, make(chan struct{}))
	// 创建informerFactory 及 mpa target ref选择器 fetcher
	kubeClient := kubernetes.NewForConfigOrDie(kubeconfig)
	informerFactory := informers.NewSharedInformerFactory(kubeClient, defaultResyncPeriod)
	mpaTargetSelectorFetcher := target.NewMpaTargetSelectorFetcher(kubeconfig, kubeClient, informerFactory)

	// 创建 recommendation 获取器
	limitRangeCalculator, err := limitrange.NewCalculator(informerFactory)
	if err != nil {
		klog.Errorf("failed to create limitRangeCalculator, err: %v", err)
	}
	recommedendationProcessor := recommendation.NewProcessor(limitRangeCalculator)
	recommendationProvider := admissionUtil.NewRecommendationProvider(limitRangeCalculator, recommedendationProcessor)

	// 创建mpa matcher & patchesCalculators
	mpaMatcher := mpaUtil.NewMatcher(mpaLister, mpaTargetSelectorFetcher)
	patchesCalculators := []admissionUtil.PatchCalculator{
		podPatch.NewObservedPodPatchCalculator(),
		podPatch.NewResourceUpdatesPatchCalculator(recommendationProvider),
	}

	admissionServer := NewAdmissionServer(mpaMatcher, patchesCalculators)
	http.HandleFunc("/", admissionServer.Serve)

	webhookServer := &http.Server{
		Addr:      fmt.Sprintf(":%d", *port),
		TLSConfig: configTLS(kubeClient, certs.serverCert, certs.serverKey),
	}

	go webhookRegistration(kubeClient, certs.caCert, namespace, *serviceName, "", false, int32(*webhookTimeout))
	webhookServer.ListenAndServeTLS("", "")
}
