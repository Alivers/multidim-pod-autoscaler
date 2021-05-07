package main

import (
	"bytes"
	"encoding/binary"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog"
	"math"
	"net/http"
	"strconv"
	"time"
)

var (
	httpRequestsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "统计该应用的http请求数",
		},
	)
)

// InitializeMetrics 初始化 prometheus 处理 "/metrics" 的请求
func initializeMetrics(address string) {
	registry := prometheus.NewRegistry()
	registry.MustRegister(httpRequestsTotal)

	go func() {
		// 注册 "/metrics" 请求 的 handler
		http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
		// "/" 地址的请求 handler 为空
		err := http.ListenAndServe(address, nil)
		klog.Fatalf("Error occured while start metrics: %v", err)
	}()
}

func serve(w http.ResponseWriter, r *http.Request) {
	httpRequestsTotal.Inc()

	x := 0.0001
	for i := 0; i <= 10000000; i += 1 {
		x += math.Sqrt(x)
	}
	buf := bytes.NewBuffer([]byte{})
	binary.Write(buf, binary.BigEndian, &x)

	xStr := "time: " + time.Now().String() + " | result: " + strconv.FormatFloat(x, 'f', -1, 64) + "\n"

	if _, err := w.Write([]byte(xStr)); err != nil {
		klog.Error(err)
	}
}

func main() {
	// prometheus metrics api
	initializeMetrics(":8081")

	// 服务接口
	http.HandleFunc("/", serve)
	http.ListenAndServe(":8082", nil)
}
