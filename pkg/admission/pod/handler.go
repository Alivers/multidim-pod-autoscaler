package pod

import (
	"encoding/json"
	"fmt"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	admissionUtil "multidim-pod-autoscaler/pkg/admission/util"
	mpaApi "multidim-pod-autoscaler/pkg/util/mpa"
	"multidim-pod-autoscaler/pkg/util/patch"
)

type podHandler struct {
	mapMatcher       mpaApi.Matcher
	patchCalculators []admissionUtil.PatchCalculator
}

// AdmissionResource 获取此handler可以处理的资源类型
func (ph *podHandler) AdmissionResource() admissionUtil.AdmissionResource {
	return admissionUtil.Pod
}

// GroupResource 获取此handler可以处理的 Group Resource
func (ph *podHandler) GroupResource() metaV1.GroupResource {
	return metaV1.GroupResource{
		Group:    "",
		Resource: "pods",
	}
}

func (ph *podHandler) GetPatches(ar *v1beta1.AdmissionRequest) ([]patch.Patch, error) {
	if ar.Resource.Version != "v1" {
		return nil, fmt.Errorf("only v1 pods are supported")
	}

	rawData, namespace := ar.Object.Raw, ar.Namespace
	pod := corev1.Pod{}

	if err := json.Unmarshal(rawData, &pod); err != nil {
		return nil, err
	}
}
