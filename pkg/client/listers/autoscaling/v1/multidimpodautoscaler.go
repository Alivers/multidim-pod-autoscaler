/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by lister-gen. DO NOT EDIT.

package v1

import (
	v1 "multidim-pod-autoscaler/pkg/apis/autoscaling/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// MultidimPodAutoscalerLister helps list MultidimPodAutoscalers.
// All objects returned here must be treated as read-only.
type MultidimPodAutoscalerLister interface {
	// List lists all MultidimPodAutoscalers in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.MultidimPodAutoscaler, err error)
	// MultidimPodAutoscalers returns an object that can list and get MultidimPodAutoscalers.
	MultidimPodAutoscalers(namespace string) MultidimPodAutoscalerNamespaceLister
	MultidimPodAutoscalerListerExpansion
}

// multidimPodAutoscalerLister implements the MultidimPodAutoscalerLister interface.
type multidimPodAutoscalerLister struct {
	indexer cache.Indexer
}

// NewMultidimPodAutoscalerLister returns a new MultidimPodAutoscalerLister.
func NewMultidimPodAutoscalerLister(indexer cache.Indexer) MultidimPodAutoscalerLister {
	return &multidimPodAutoscalerLister{indexer: indexer}
}

// List lists all MultidimPodAutoscalers in the indexer.
func (s *multidimPodAutoscalerLister) List(selector labels.Selector) (ret []*v1.MultidimPodAutoscaler, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.MultidimPodAutoscaler))
	})
	return ret, err
}

// MultidimPodAutoscalers returns an object that can list and get MultidimPodAutoscalers.
func (s *multidimPodAutoscalerLister) MultidimPodAutoscalers(namespace string) MultidimPodAutoscalerNamespaceLister {
	return multidimPodAutoscalerNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// MultidimPodAutoscalerNamespaceLister helps list and get MultidimPodAutoscalers.
// All objects returned here must be treated as read-only.
type MultidimPodAutoscalerNamespaceLister interface {
	// List lists all MultidimPodAutoscalers in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.MultidimPodAutoscaler, err error)
	// Get retrieves the MultidimPodAutoscaler from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1.MultidimPodAutoscaler, error)
	MultidimPodAutoscalerNamespaceListerExpansion
}

// multidimPodAutoscalerNamespaceLister implements the MultidimPodAutoscalerNamespaceLister
// interface.
type multidimPodAutoscalerNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all MultidimPodAutoscalers in the indexer for a given namespace.
func (s multidimPodAutoscalerNamespaceLister) List(selector labels.Selector) (ret []*v1.MultidimPodAutoscaler, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.MultidimPodAutoscaler))
	})
	return ret, err
}

// Get retrieves the MultidimPodAutoscaler from the indexer for a given namespace and name.
func (s multidimPodAutoscalerNamespaceLister) Get(name string) (*v1.MultidimPodAutoscaler, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("multidimpodautoscaler"), name)
	}
	return obj.(*v1.MultidimPodAutoscaler), nil
}
