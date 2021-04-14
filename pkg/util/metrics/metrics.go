package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog"
	"net/http"
	"time"
)

// InitializeMetrics 初始化 prometheus 处理 "/metrics" 的请求
func InitializeMetrics(address string) {
	go func() {
		// 注册 "/metrics" 请求 的 handler
		http.Handle("/metrics", promhttp.Handler())
		// "/" 地址的请求 handler 为空
		err := http.ListenAndServe(address, nil)
		klog.Fatalf("Error occured while start metrics: %v", err)
	}()
}

// CreateExecutionTimeMetric 创建一个新的 执行时间 度量直方图序列，一个histogram对应Buckets中的一个上界
// 对应 histogram 只在创建响应的 Observation 时由prometheus自动创建
func CreateExecutionTimeMetric(namespace, help string) *prometheus.HistogramVec {
	return prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "exection_latency_seconds",
			Help:      help,
			Buckets: []float64{0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1.0, 2.0, 5.0, 10.0,
				20.0, 30.0, 40.0, 50.0, 60.0, 70.0, 80.0, 90.0, 100.0, 120.0, 150.0, 240.0, 300.0},
		},
		[]string{"step"},
	)
}

const (
	// MetricsNamespace 为MPA组件的 metrics 命名空间前缀(用于prometheus)
	MetricsNamespace string = "mpa_"
)

// ExecutionTimer 用于测量 某个步骤 的执行时间
// 主要分为以下几步:
// 1. timer := NewExecutionTimer(...)
// 2. 被测量程序
// 3. timer.ObserveStep()
// ...
// [n]. timer.ObserverTotal()
type ExecutionTimer struct {
	histogram *prometheus.HistogramVec
	start     time.Time
	end       time.Time
}

// NewExecutionTimer 创建一个新的 ExecutionTimer(创建时间即为 start 开始测量)
func NewExecutionTimer(histogram *prometheus.HistogramVec) *ExecutionTimer {
	now := time.Now()
	return &ExecutionTimer{
		histogram: histogram,
		start:     now,
		end:       now,
	}
}

func (timer *ExecutionTimer) ObserveStep(step string) {
	now := time.Now()
	// 观测从上一个 step的 end 到 now 花费时间 落在 histogram 的哪个 bucket 中
	timer.histogram.WithLabelValues(step).Observe(now.Sub(timer.end).Seconds())
	// 更新end时间，用于下次 step 的测量
	timer.end = now
}

func (timer *ExecutionTimer) ObserveTotal() {
	// 观测从 ExecutionTimer 创建开始到 Now 的时间
	timer.histogram.WithLabelValues("total").Observe(time.Now().Sub(timer.start).Seconds())
}
