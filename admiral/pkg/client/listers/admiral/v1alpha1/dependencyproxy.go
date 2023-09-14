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

// DependencyProxyLister helps list DependencyProxies.
// All objects returned here must be treated as read-only.
type DependencyProxyLister interface {
	// List lists all DependencyProxies in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.DependencyProxy, err error)
	// DependencyProxies returns an object that can list and get DependencyProxies.
	DependencyProxies(namespace string) DependencyProxyNamespaceLister
	DependencyProxyListerExpansion
}

// dependencyProxyLister implements the DependencyProxyLister interface.
type dependencyProxyLister struct {
	indexer cache.Indexer
}

// NewDependencyProxyLister returns a new DependencyProxyLister.
func NewDependencyProxyLister(indexer cache.Indexer) DependencyProxyLister {
	return &dependencyProxyLister{indexer: indexer}
}

// List lists all DependencyProxies in the indexer.
func (s *dependencyProxyLister) List(selector labels.Selector) (ret []*v1alpha1.DependencyProxy, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.DependencyProxy))
	})
	return ret, err
}

// DependencyProxies returns an object that can list and get DependencyProxies.
func (s *dependencyProxyLister) DependencyProxies(namespace string) DependencyProxyNamespaceLister {
	return dependencyProxyNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// DependencyProxyNamespaceLister helps list and get DependencyProxies.
// All objects returned here must be treated as read-only.
type DependencyProxyNamespaceLister interface {
	// List lists all DependencyProxies in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.DependencyProxy, err error)
	// Get retrieves the DependencyProxy from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.DependencyProxy, error)
	DependencyProxyNamespaceListerExpansion
}

// dependencyProxyNamespaceLister implements the DependencyProxyNamespaceLister
// interface.
type dependencyProxyNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all DependencyProxies in the indexer for a given namespace.
func (s dependencyProxyNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.DependencyProxy, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.DependencyProxy))
	})
	return ret, err
}

// Get retrieves the DependencyProxy from the indexer for a given namespace and name.
func (s dependencyProxyNamespaceLister) Get(name string) (*v1alpha1.DependencyProxy, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("dependencyproxy"), name)
	}
	return obj.(*v1alpha1.DependencyProxy), nil
}
