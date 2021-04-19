package util

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

// CreateKubeConfig 构造kubeconfig
// kubeconfig 为空时，使用 in-cluster 配置
// kubeconfig 不为空，使用指定配置(集群外部)
func CreateKubeConfig(kubeconfig string, kubeApiQps float32, kubeApiBurst int) *rest.Config {
	var config *rest.Config
	var err error
	if len(kubeconfig) > 0 {
		klog.V(1).Infof("Using kubeconfig file: %s", kubeconfig)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			klog.Fatalf("Failed to build kubeconfig from file: %v", err)
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			klog.Fatalf("Failed to create config: %v", err)
		}
	}

	config.QPS = kubeApiQps
	config.Burst = kubeApiBurst
	return config
}
