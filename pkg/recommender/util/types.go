package util

import corev1 "k8s.io/api/core/v1"

type MpaId struct {
	Namespace string
	Name      string
}

type PodId struct {
	Namespace string
	Name      string
}

type ContainerId struct {
	PodId PodId
	Name  string
}

func GetPodId(pod *corev1.Pod) PodId {
	if pod != nil {
		return PodId{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		}
	}
	return PodId{}
}
