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

package v1alpha1

import (
	v1alpha1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// OutlierDetectionLister helps list OutlierDetections.
// All objects returned here must be treated as read-only.
type OutlierDetectionLister interface {
	// List lists all OutlierDetections in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.OutlierDetection, err error)
	// OutlierDetections returns an object that can list and get OutlierDetections.
	OutlierDetections(namespace string) OutlierDetectionNamespaceLister
	OutlierDetectionListerExpansion
}

// outlierDetectionLister implements the OutlierDetectionLister interface.
type outlierDetectionLister struct {
	indexer cache.Indexer
}

// NewOutlierDetectionLister returns a new OutlierDetectionLister.
func NewOutlierDetectionLister(indexer cache.Indexer) OutlierDetectionLister {
	return &outlierDetectionLister{indexer: indexer}
}

// List lists all OutlierDetections in the indexer.
func (s *outlierDetectionLister) List(selector labels.Selector) (ret []*v1alpha1.OutlierDetection, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.OutlierDetection))
	})
	return ret, err
}

// OutlierDetections returns an object that can list and get OutlierDetections.
func (s *outlierDetectionLister) OutlierDetections(namespace string) OutlierDetectionNamespaceLister {
	return outlierDetectionNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// OutlierDetectionNamespaceLister helps list and get OutlierDetections.
// All objects returned here must be treated as read-only.
type OutlierDetectionNamespaceLister interface {
	// List lists all OutlierDetections in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.OutlierDetection, err error)
	// Get retrieves the OutlierDetection from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.OutlierDetection, error)
	OutlierDetectionNamespaceListerExpansion
}

// outlierDetectionNamespaceLister implements the OutlierDetectionNamespaceLister
// interface.
type outlierDetectionNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all OutlierDetections in the indexer for a given namespace.
func (s outlierDetectionNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.OutlierDetection, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.OutlierDetection))
	})
	return ret, err
}

// Get retrieves the OutlierDetection from the indexer for a given namespace and name.
func (s outlierDetectionNamespaceLister) Get(name string) (*v1alpha1.OutlierDetection, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("outlierdetection"), name)
	}
	return obj.(*v1alpha1.OutlierDetection), nil
}