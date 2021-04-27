package util

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
