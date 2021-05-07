package config

import (
	"context"
	"crypto/tls"
	"time"

	admissionregistration "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
)

const (
	webhookConfigName = "mpa-webhook-config"
)

// GetClient 使用 k8s集群内部配置构造并返回 clientset
func GetClient() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatal(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatal(err)
	}
	return clientset
}

// ConfigTLS 配置https证书链
func ConfigTLS(clientset *kubernetes.Clientset, serverCert, serverKey []byte) *tls.Config {
	sCert, err := tls.X509KeyPair(serverCert, serverKey)
	if err != nil {
		klog.Fatal(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{sCert},
	}
}

// WebhookRegistration api-server 注册 admission控制器的webhook配置
func WebhookRegistration(clientset *kubernetes.Clientset, caCert []byte, namespace, serviceName, url string, registerByURL bool, timeoutSeconds int32) {
	time.Sleep(10 * time.Second)
	webhookClient := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations()
	_, err := webhookClient.Get(context.TODO(), webhookConfigName, metav1.GetOptions{})
	if err == nil {
		// 同名webhook已经被配置，先删除后继续配置
		if err2 := webhookClient.Delete(context.TODO(), webhookConfigName, metav1.DeleteOptions{}); err2 != nil {
			klog.Fatal(err2)
		}
	}
	RegisterClientConfig := admissionregistration.WebhookClientConfig{}
	// url 和 service 必须指定一个
	if !registerByURL {
		RegisterClientConfig.Service = &admissionregistration.ServiceReference{
			Namespace: namespace,
			Name:      serviceName,
		}
	} else {
		RegisterClientConfig.URL = &url
	}
	// 无副作用(如：在回调处理中修改资源等)
	sideEffects := admissionregistration.SideEffectClassNone
	// 忽略webhook的错误调用
	failurePolicy := admissionregistration.Ignore
	// tls连接的证书(webhook server与 api-server之间通信)
	RegisterClientConfig.CABundle = caCert
	webhookConfig := &admissionregistration.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: webhookConfigName,
		},
		Webhooks: []admissionregistration.MutatingWebhook{
			{
				Name:                    "mpa.k8s.io",
				AdmissionReviewVersions: []string{"v1"},
				// webhook 的操作规则
				Rules: []admissionregistration.RuleWithOperations{
					// 拦截创建Pod的请求
					{
						Operations: []admissionregistration.OperationType{admissionregistration.Create},
						Rule: admissionregistration.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"pods"},
						},
					},
					// 拦截创建、更新MPA对象的请求
					//{
					//	Operations: []admissionregistration.OperationType{admissionregistration.Create, admissionregistration.Update},
					//	Rule: admissionregistration.Rule{
					//		APIGroups:   []string{"autoscaling.k8s.io"},
					//		APIVersions: []string{"*"},
					//		Resources:   []string{"multidimpodautoscalers"},
					//	},
					//},
				},
				FailurePolicy:  &failurePolicy,
				ClientConfig:   RegisterClientConfig,
				SideEffects:    &sideEffects,
				TimeoutSeconds: &timeoutSeconds,
			},
		},
	}
	if _, err := webhookClient.Create(context.TODO(), webhookConfig, metav1.CreateOptions{}); err != nil {
		klog.Fatal(err)
	} else {
		klog.V(3).Info("Webhook registration as MutatingWebhook succeeded.")
	}
}
