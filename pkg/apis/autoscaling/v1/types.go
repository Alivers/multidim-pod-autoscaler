package v1

import (
	autoscaling "k8s.io/api/autoscaling/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MultidimPodAutoscalerList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list metadata.
	// 匿名字段
	// +optional
	metav1.ListMeta `json:"metadata" protobuf:"bytes,1,opt,name=metadata"`

	// items is the list of multidim pod autoscaler objects.
	Items []MultidimPodAutoscaler `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=vpa

// 伸缩器的状态信息 用于自动伸缩
type MultidimPodAutoscaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// 伸缩器的配置
	Spec MultidimPodAutoscalerSpec `json:"spec" protobuf:"bytes,2,name=spec"`

	// 伸缩器的当前状态信息
	// +optional
	Status MultidimPodAutoscalerStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// 伸缩器的配置结构
type MultidimPodAutoscalerSpec struct {

	// TargetRef 指向管理POD集合来实现自动伸缩控制的控制器(deployment、statefulSet)
	TargetRef *autoscaling.CrossVersionObjectReference `json:"targetRef" protobuf:"bytes,1,name=targetRef"`

	// 针对POD如何改变(资源等)的策略描述
	// 未指定则使用默认值
	// +optional
	UpdatePolicy *PodUpdatePolicy `json:"updatePolicy,omitempty" protobuf:"bytes,2,opt,name=updatePolicy"`

	// 伸缩算法中需要考虑的一些用户配置(资源上下限等)
	// 未指定时，将默认算法应用到全部容器(计算伸缩方案)
	// +optional
	ResourcePolicy *PodResourcePolicy `json:"resourcePolicy,omitempty" protobuf:"bytes,3,opt,name=resourcePolicy"`
}

// 针对POD如何改变(资源等)的策略描述
type PodUpdatePolicy struct {
	// POD的更新策略
	// 默认为 'Auto'.
	// +optional
	UpdateMode *UpdateMode `json:"updateMode,omitempty" protobuf:"bytes,1,opt,name=updateMode"`
}

// 伸缩器针对POD的更新模式
// +kubebuilder:validation:Enum=Off;Initial;Recreate;Auto
type UpdateMode string

const (
	// off模式下伸缩器不会尝试改变POD的资源
	// 此模式下伸缩算法还是会继续执行，但是方案不应用到POD
	UpdateModeOff UpdateMode = "Off"
	// auto模式下：创建POD 和 POD运行过程中 均应用方案(重建POD)
	UpdateModeAuto UpdateMode = "Auto"
)

// P伸缩算法中需要考虑的一些用户配置(资源上下限等)
// 可使用 `containerName` = '*' 标识全部容器
// 单个容器需要指定容器的唯一name
type PodResourcePolicy struct {
	// 每个容器的资源策略
	// +optional
	// +patchMergeKey=containerName
	// +patchStrategy=merge
	ContainerPolicies []ContainerResourcePolicy `json:"containerPolicies,omitempty" patchStrategy:"merge" patchMergeKey:"containerName" protobuf:"bytes,1,rep,name=containerPolicies"`
}

// 容器的资源策略配置(用户预配置)
type ContainerResourcePolicy struct {
	// 容器名('*' 通配表示全部)
	ContainerName string `json:"containerName,omitempty" protobuf:"bytes,1,opt,name=containerName"`
	// 伸缩器是否要应用到该容器
	// +optional
	Mode *ContainerScalingMode `json:"mode,omitempty" protobuf:"bytes,2,opt,name=mode"`
	// 资源的下限限制(默认无限制)
	// +optional
	MinAllowed v1.ResourceList `json:"minAllowed,omitempty" protobuf:"bytes,3,rep,name=minAllowed,casttype=ResourceList,castkey=ResourceName"`
	// 资源的上限限制(默认无限制)
	// +optional
	MaxAllowed v1.ResourceList `json:"maxAllowed,omitempty" protobuf:"bytes,4,rep,name=maxAllowed,casttype=ResourceList,castkey=ResourceName"`
	// 请求的预期响应时间
	// +optional
	ExpRespTime int `json:"expRespTime,omitempty" protobuf:"int32,5,req,name=expRespTime"`
}

const (
	// 默认容器资源策略为全部容器应用
	DefaultContainerResourcePolicy = "*"
)

// 自动伸缩器是否应用到容器
// +kubebuilder:validation:Enum=Auto;Off
type ContainerScalingMode string

const (
	// 应用到容器
	ContainerScalingModeAuto ContainerScalingMode = "Auto"
	// 不应用
	ContainerScalingModeOff ContainerScalingMode = "Off"
)

// 描述伸缩器的运行状态
type MultidimPodAutoscalerStatus struct {
	// 最新的资源配置方案
	// +optional
	RecommendationResources *RecommendedResources `json:"recommendationResource,omitempty" protobuf:"bytes,1,opt,name=recommendationResource"`

	// 伸缩器用于伸缩的条件(判断条件是否满足)
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []MultidimPodAutoscalerCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,3,rep,name=conditions"`
}

// 伸缩器计算得出的伸缩方案
type RecommendedResources struct {
	// 伸缩算法得出的资源方案(每个Pod的资源量和Pod的个数)
	TargetResource v1.ResourceList `json:"targetResource" protobuf:"bytes,1,rep,name=targetResource,casttype=ResourceList,castkey=ResourceName"`

	TargetPodNum int `json:"targetPodNum" protobuf:"int32,2,opt,name=targetPodNum"`

	// 算法中间结果 保证方案不是最差选择(资源量下限)
	// +optional
	LowerBoundResource v1.ResourceList `json:"lowerBoundResource" protobuf:"bytes,1,rep,name=lowerBoundResource,casttype=ResourceList,castkey=ResourceName"`
	// +optional
	LowerBoundPodNum int `json:"lowerBoundPodNum" protobuf:"int32,2,opt,name=lowerBoundPodNum"`

	// 算法中间结果 保证方案不是最差选择(资源量上限)
	// +optional
	UpperBoundResource v1.ResourceList `json:"upperBoundResource" protobuf:"bytes,1,rep,name=upperBoundResource,casttype=ResourceList,castkey=ResourceName"`
	// +optional
	UpperBoundPodNum int `json:"upperBoundPodNum" protobuf:"int32,2,opt,name=upperBoundPodNum"`

	// Target是考虑了 ContainerResourcePolicy 的方案
	// UncappedTarget未考虑该限制(即无界)
	// 仅用于状态描述，不会实际应用
	// +optional
	UncappedTargetResource v1.ResourceList `json:"uncappedTargetResource" protobuf:"bytes,1,rep,name=uncappedTargetResource,casttype=ResourceList,castkey=ResourceName"`
	// +optional
	UncappedTargetPodNum int `json:"uncappedTargetPodNum" protobuf:"int32,2,opt,name=uncappedTargetPodNum"`
}

// 伸缩器的合法状态
type MultidimPodAutoscalerConditionType string

var (
	// 伸缩器可以进行方案计算及推荐
	RecommendationProvided MultidimPodAutoscalerConditionType = "RecommendationProvided"
	// label selector未匹配到POD
	NoPodsMatched MultidimPodAutoscalerConditionType = "NoPodsMatched"
)

// 伸缩器在某时刻的状态
type MultidimPodAutoscalerCondition struct {
	// 状态描述
	Type MultidimPodAutoscalerConditionType `json:"type" protobuf:"bytes,1,name=type"`
	// 状态 (True, False, Unknown)
	Status v1.ConditionStatus `json:"status" protobuf:"bytes,2,name=status"`
	// 上次状态改变的时间
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,3,opt,name=lastTransitionTime"`
	// 上次状态改变的原因
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,4,opt,name=reason"`
	// 可读的状态改变原因
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,5,opt,name=message"`
}
