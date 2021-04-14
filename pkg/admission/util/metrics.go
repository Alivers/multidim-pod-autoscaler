// metics of admission

package util

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"multidim-pod-autoscaler/pkg/util/metrics"
	"time"
)

const (
	// metricsNamespace admission metrics的namespace
	// 用于 prometheus metrics
	// 本包私有，不向外暴露
	metricsNamespace string = metrics.MetricsNamespace + "admission"
)

// AdmissionStatus 表示admission的状态(webhook回调处理过程中的状态)
type AdmissionStatus string

// AdmissionResource 表示admission处理的资源(webhook回调中请求指定操作的资源)
type AdmissionResource string

const (
	// Error 错误状态
	Error AdmissionStatus = "error"
	// Skipped 表示该请求被忽略
	Skipped AdmissionStatus = "skipped"
	// Applied 表示该请求已被应用到指定的资源
	Applied AdmissionStatus = "applied"
)

const (
	// Unknown 表示指定资源无法识别
	Unknown AdmissionResource = "unknown"
	// Pod 请求操作pod
	Pod AdmissionResource = "pod"
	// Mpa 请求操作mpa object
	Mpa AdmissionResource = "mpa"
)

var (
	admissionPodCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "admission_pods_total",
			Help:      "MPA Admission 处理的 Pod 总数",
		},
		[]string{string(Applied)},
	)
	admissionLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "admission_latency_seconds",
			Help:      "MPA Admission 中某个过程花费的时间(延迟)",
			Buckets:   []float64{0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1.0, 2.0, 5.0, 10.0, 20.0, 30.0, 60.0, 120.0, 300.0},
		}, []string{"status", "resource"},
	)
)

// RegisterMetrics 初始化 admission 需要用到的 metrics
func RegisterMetrics() {
	prometheus.MustRegister(admissionPodCount, admissionLatency)
}

// OnAppliedPod 更新admission处理的pod的计数器
// 应用成功的才计数
// 首次调用, 会创建 label为 "true" 的 Counter
func OnAppliedPod(applied bool) {
	admissionPodCount.WithLabelValues(fmt.Sprintf("%v", applied)).Add(1)
}

// AdmissionLatencyTimer 测量 admission 中某过程的执行或延迟时间
// 分为以下几步:
// 1. timer := NewAdmissionLatencyTimer()
// 2. 被测量代码
// 3. timer.Observe(...)
type AdmissionLatencyTimer struct {
	histo *prometheus.HistogramVec
	// start 为AdmissionLatencyTimer的创建时间
	// 即自AdmissionLatencyTimer创建开始计时测量
	start time.Time
	// end 暂时用不到，先留着吧
	end time.Time
}

// NewAdmissionLatencyTimer 创建一个新的 AdmissionLatencyTimer
// 创建即开始计时
func NewAdmissionLatencyTimer() *AdmissionLatencyTimer {
	return &AdmissionLatencyTimer{
		histo: admissionLatency,
		start: time.Now(),
	}
}

// Observe 测量从 AdmissionLatencyTimer 创建到 Now 的用时
func (t *AdmissionLatencyTimer) Observe(status AdmissionStatus, resource AdmissionResource) {
	t.histo.WithLabelValues(string(status), string(resource)).Observe(time.Now().Sub(t.start).Seconds())
}
