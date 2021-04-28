package util

import (
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	cachedDiscovery "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/metrics/pkg/client/custom_metrics"
	"time"
)

const (
	// discovery client 缓存重置的时间间隔
	discoveryResetPeriod = 5 * time.Minute
)

// NewCustomMetricsClient 返回一个新的 CustomMetricsClient
func NewCustomMetricsClient(config *rest.Config) custom_metrics.CustomMetricsClient {
	discoveryClient := discovery.NewDiscoveryClientForConfigOrDie(config)
	cachedDiscoveryClient := cachedDiscovery.NewMemCacheClient(discoveryClient)
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedDiscoveryClient)

	go wait.Until(func() {
		restMapper.Reset()
	}, discoveryResetPeriod, make(chan struct{}))

	apiVersionGetter := custom_metrics.NewAvailableAPIsGetter(discoveryClient)
	customClient := custom_metrics.NewForConfig(config, restMapper, apiVersionGetter)

	return customClient
}
