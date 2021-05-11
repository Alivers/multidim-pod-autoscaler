package pod

import (
	"encoding/json"
	"fmt"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	podPatch "multidim-pod-autoscaler/pkg/admission/pod/patch"
	admissionUtil "multidim-pod-autoscaler/pkg/admission/util"
	mpaApi "multidim-pod-autoscaler/pkg/util/mpa"
	patchUtil "multidim-pod-autoscaler/pkg/util/patch"
)

type podHandler struct {
	mpaMatcher       mpaApi.Matcher
	patchCalculators []admissionUtil.PatchCalculator
}

// NewPodHandler 返回一个新的 pod handler 用于计算pod的patch
func NewPodHandler(mpaMatcher mpaApi.Matcher, patchCalculators []admissionUtil.PatchCalculator) admissionUtil.Handler {
	return &podHandler{
		mpaMatcher:       mpaMatcher,
		patchCalculators: patchCalculators,
	}
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

// GetPatches 实现 handler接口，用于计算 admission request中指定的pod的patches
func (ph *podHandler) GetPatches(ar *v1beta1.AdmissionRequest) ([]patchUtil.Patch, error) {
	if ar.Resource.Version != "v1" {
		return nil, fmt.Errorf("only v1 pods are supported")
	}

	rawData, namespace := ar.Object.Raw, ar.Namespace
	pod := corev1.Pod{}

	if err := json.Unmarshal(rawData, &pod); err != nil {
		return nil, err
	}

	if len(pod.Name) == 0 {
		pod.Name = pod.GenerateName + "%"
		pod.Namespace = namespace
	}

	klog.V(4).Infof("Admitting Pod: name=%s,namespace=%s,generateName=%s", pod.Name, pod.Namespace, pod.GenerateName)

	// 获取控制该pod的MPA 对象
	controllingMpa := ph.mpaMatcher.GetPodMatchingMpa(&pod)
	if controllingMpa == nil {
		klog.V(4).Infof("No Matching MPA found for pod %s-%s", pod.Namespace, pod.Name)
		return []patchUtil.Patch{}, nil
	}

	patches := make([]patchUtil.Patch, 0)
	if pod.Annotations == nil {
		patches = append(patches, podPatch.GetEmptyAddAnnotationsPatch())
	}
	// 计算pod的修改patch
	for _, calculator := range ph.patchCalculators {
		subPatches, err := calculator.CalculatePatches(&pod, controllingMpa)
		if err != nil {
			return []patchUtil.Patch{}, err
		}
		patches = append(patches, subPatches...)
	}

	return patches, nil
}
