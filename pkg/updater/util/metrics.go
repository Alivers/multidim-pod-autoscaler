package util

import (
	"github.com/prometheus/client_golang/prometheus"
	"multidim-pod-autoscaler/pkg/util/metrics"
)

const (
	metricsNamespace = metrics.TopNamespace + "updater"
)

var (
	updaterLatency = metrics.CreateExecutionTimeMetric(metricsNamespace, "mpa updater主流程中的执行时间")
)

func RegisterMetrics() {
	prometheus.MustRegister(updaterLatency)
}

func NewExecutionTimer() *metrics.ExecutionTimer {
	return metrics.NewExecutionTimer(updaterLatency)
}
