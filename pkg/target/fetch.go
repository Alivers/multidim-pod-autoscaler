package target

import (
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/scale"

	mpa_types "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"

	kube_client "k8s.io/client-go/kubernetes"
)

const (
	discoveryResetPeriod time.Duration = 5 * time.Minute
)

type MpaTargetSelectorFetch interface {
	Fetch(mpa *mpa_types.MultidimPodAutoscaler) (labels.Selector, error)
}

type wellKnownController string

const (
	daemonSet             wellKnownController = "DaemonSet"
	deployment            wellKnownController = "Deployment"
	replicaSet            wellKnownController = "ReplicaSet"
	statefulSet           wellKnownController = "StatefulSet"
	replicationController wellKnownController = "ReplicationController"
	job                   wellKnownController = "Job"
	cronJob               wellKnownController = "CronJob"
)

func NewMpaTargetSelectorFetcher(config *rest.Config, kubeClient kube_client.Interface, factory informers.SharedInformerFactory) MpaTargetSelectorFetch {

}

type mpaTargetSelectorFetch struct {
	scalerNamespacer scale.ScalesGetter
	mapper           apimeta.RESTMapper
}
