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

// DependencyLister helps list Dependencies.
// All objects returned here must be treated as read-only.
type DependencyLister interface {
	// List lists all Dependencies in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.Dependency, err error)
	// Dependencies returns an object that can list and get Dependencies.
	Dependencies(namespace string) DependencyNamespaceLister
	DependencyListerExpansion
}

// dependencyLister implements the DependencyLister interface.
type dependencyLister struct {
	indexer cache.Indexer
}

// NewDependencyLister returns a new DependencyLister.
func NewDependencyLister(indexer cache.Indexer) DependencyLister {
	return &dependencyLister{indexer: indexer}
}

// List lists all Dependencies in the indexer.
func (s *dependencyLister) List(selector labels.Selector) (ret []*v1alpha1.Dependency, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Dependency))
	})
	return ret, err
}

// Dependencies returns an object that can list and get Dependencies.
func (s *dependencyLister) Dependencies(namespace string) DependencyNamespaceLister {
	return dependencyNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// DependencyNamespaceLister helps list and get Dependencies.
// All objects returned here must be treated as read-only.
type DependencyNamespaceLister interface {
	// List lists all Dependencies in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.Dependency, err error)
	// Get retrieves the Dependency from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.Dependency, error)
	DependencyNamespaceListerExpansion
}

// dependencyNamespaceLister implements the DependencyNamespaceLister
// interface.
type dependencyNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all Dependencies in the indexer for a given namespace.
func (s dependencyNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.Dependency, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Dependency))
	})
	return ret, err
}

// Get retrieves the Dependency from the indexer for a given namespace and name.
func (s dependencyNamespaceLister) Get(name string) (*v1alpha1.Dependency, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("dependency"), name)
	}
	return obj.(*v1alpha1.Dependency), nil
}
