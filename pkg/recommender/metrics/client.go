package metrics

import (
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/metrics/pkg/client/custom_metrics"
	recommenderUtil "multidim-pod-autoscaler/pkg/recommender/util"
	"time"
)

const (
	metricServerDefaultMetricWindow = time.Minute
)

// PodMetric 描述了pod的metrics信息
type PodMetric struct {
	// metrics指标的name
	Name string
	// 表示在某一时刻 Metrics 信息被获取
	Timestamp time.Time
	// metrics 时间窗口 [Timestamp - Window, Timestamp] 区间
	Window time.Duration
	// metrics 的值
	Value resource.Quantity
}

// PodMetricsInfo 为 pod - 其Metrics信息的映射
type PodMetricsInfo map[recommenderUtil.PodId][]PodMetric

// Client 提供了获取自定义pod metrics指标(如：qps等)的接口
type Client interface {
	// GetPodRawMetric 获取namespace下匹配selector的所有pod的metricsName对应的metrics信息
	GetPodRawMetric(metricName string, namespace string, selector labels.Selector, metricSelector labels.Selector) (PodMetricsInfo, time.Time, error)
}

type customClient struct {
	client custom_metrics.CustomMetricsClient
}

func NewClient(cmClient custom_metrics.CustomMetricsClient) Client {
	return &customClient{
		client: cmClient,
	}
}

func (c *customClient) GetPodRawMetric(
	metricName string,
	namespace string,
	selector labels.Selector,
	metricSelector labels.Selector,
) (PodMetricsInfo, time.Time, error) {
	metrics, err := c.client.NamespacedMetrics(namespace).GetForObjects(schema.GroupKind{Kind: "Pod"}, selector, metricName, metricSelector)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("unable to fetch metrics from custom metrics API: %v", err)
	}

	if len(metrics.Items) == 0 {
		return nil, time.Time{}, fmt.Errorf("no metrics returned from custom metrics API")
	}

	res := make(PodMetricsInfo, len(metrics.Items))
	for _, m := range metrics.Items {
		window := metricServerDefaultMetricWindow
		if m.WindowSeconds != nil {
			window = time.Duration(*m.WindowSeconds) * time.Second
		}
		podId := recommenderUtil.PodId{
			Namespace: m.DescribedObject.Namespace,
			Name:      m.DescribedObject.Name,
		}
		res[podId] = []PodMetric{
			{
				Name:      metricName,
				Timestamp: m.Timestamp.Time,
				Window:    window,
				Value:     m.Value,
			},
		}
	}

	timestamp := metrics.Items[0].Timestamp.Time

	return res, timestamp, nil
}
