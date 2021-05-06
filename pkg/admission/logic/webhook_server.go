package logic

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	"multidim-pod-autoscaler/pkg/admission/pod"
	admissionUtil "multidim-pod-autoscaler/pkg/admission/util"
	mpaApi "multidim-pod-autoscaler/pkg/util/mpa"
	patchUtil "multidim-pod-autoscaler/pkg/util/patch"
	"net/http"
)

type AdmissionServer struct {
	resourcesHandler map[metav1.GroupResource]admissionUtil.Handler
}

// NewAdmissionServer 构造一个新的 AdmissionServer
func NewAdmissionServer(
	mpaMatcher mpaApi.Matcher,
	patchesCalculators []admissionUtil.PatchCalculator,
) *AdmissionServer {
	as := &AdmissionServer{
		resourcesHandler: map[metav1.GroupResource]admissionUtil.Handler{},
	}
	podHandler := pod.NewPodHandler(mpaMatcher, patchesCalculators)
	as.resourcesHandler[podHandler.GroupResource()] = podHandler

	return as
}

// admitting 处理给定的请求，返回处理结果(patches等)
func (as *AdmissionServer) admitting(
	data []byte,
) (*v1beta1.AdmissionResponse, admissionUtil.AdmissionStatus, admissionUtil.AdmissionResource) {
	response := &v1beta1.AdmissionResponse{}
	// 允许 admission request
	response.Allowed = true

	// 解析admission request 请求
	admissionRequest := v1beta1.AdmissionReview{}
	if err := json.Unmarshal(data, &admissionRequest); err != nil {
		klog.Errorf("connot parse the admission request: %v", err)
		return response, admissionUtil.Error, admissionUtil.Unknown
	}

	resource := admissionUtil.Unknown
	patches := make([]patchUtil.Patch, 0)
	var err error

	// 设置可接受的 Group Resource
	admissionedGroupResource := metav1.GroupResource{
		Group:    admissionRequest.Request.Resource.Group,
		Resource: admissionRequest.Request.Resource.Resource,
	}

	thisHandler, ok := as.resourcesHandler[admissionedGroupResource]
	if !ok {
		err = fmt.Errorf("cannot accept the resource type : %v", admissionedGroupResource)
	} else {
		// 获取 patches
		patches, err = thisHandler.GetPatches(admissionRequest.Request)
		resource = thisHandler.AdmissionResource()
	}

	if err != nil {
		klog.Errorf("errors occored while handling admission request: %v", err)
		return response, admissionUtil.Error, resource
	}

	status := admissionUtil.Skipped

	if len(patches) > 0 {
		klog.V(4).Infof("admission get pods' patches: %v", patches)
		// 序列化patches为 字节流数据
		plainPatches, err := json.Marshal(patches)
		if err != nil {
			klog.Errorf("connot marshal the patches %v: %v", patches, err)
			return response, admissionUtil.Error, resource
		}
		patchType := v1beta1.PatchTypeJSONPatch
		response.PatchType = &patchType
		response.Patch = plainPatches
		klog.V(4).Infof("patches ready to send: %v", patches)

		status = admissionUtil.Applied
	}

	// 统计被处理的pod的个数
	if resource == admissionUtil.Pod {
		admissionUtil.OnAppliedPod(status == admissionUtil.Applied)
	}

	return response, status, resource
}

// Serve 完成一次webhook的回调执行流程
func (as *AdmissionServer) Serve(writer http.ResponseWriter, request *http.Request) {
	timer := admissionUtil.NewAdmissionLatencyTimer()

	var body []byte
	if request.Body != nil {
		// 读取请求的body数据
		if data, err := ioutil.ReadAll(request.Body); err != nil {
			body = data
		}
	}

	// 保证请求的类型是 json
	contentType := request.Header.Get("Content-Type")
	if contentType != "application/json" {
		klog.Errorf("content-type: %s, expect application/json", contentType)
		timer.Observe(admissionUtil.Error, admissionUtil.Unknown)
		return
	}

	response, status, resource := as.admitting(body)
	admissionReview := v1beta1.AdmissionReview{
		Response: response,
	}

	// 打包回复数据
	finalResponse, err := json.Marshal(admissionReview)
	if err != nil {
		klog.Error(err)
		timer.Observe(admissionUtil.Error, resource)
		return
	}

	// 写入回复包
	if _, err := writer.Write(finalResponse); err != nil {
		klog.Error(err)
		timer.Observe(admissionUtil.Error, resource)
		return
	}
	// 统计执行数据(时间)
	timer.Observe(status, resource)
}
