package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"multidim-pod-autoscaler/pkg/util/metrics"
)

const (
	metricsNamespace = metrics.TopNamespace + "recommender"
)

var (
	recommenderLatency = metrics.CreateExecutionTimeMetric(metricsNamespace, "mpa recommender主流程中的执行时间")
)

func RegisterMetrics() {
	prometheus.MustRegister(recommenderLatency)
}

func NewExecutionTimer() *metrics.ExecutionTimer {
	return metrics.NewExecutionTimer(recommenderLatency)
}
