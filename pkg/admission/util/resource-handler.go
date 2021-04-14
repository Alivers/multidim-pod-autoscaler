package util

import (
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"multidim-pod-autoscaler/pkg/util/patch"
)

// Handler 描述了对 admission server 中资源的操作
type Handler interface {
	// GroupResource 返回 Handler 可处理的 Group 和 Resource
	GroupResource() metav1.GroupResource
	// AdmissionResource 获取 Handler 可处理的资源类型
	AdmissionResource() AdmissionResource
	// GetPatches 获取admissionRequest对应的资源patch(需要进行的操作)
	GetPatches(request *v1beta1.AdmissionRequest) ([]patch.Patch, error)
}
