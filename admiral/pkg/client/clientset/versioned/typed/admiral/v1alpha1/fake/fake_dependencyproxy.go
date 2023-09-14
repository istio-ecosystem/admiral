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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1alpha1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeDependencyProxies implements DependencyProxyInterface
type FakeDependencyProxies struct {
	Fake *FakeAdmiralV1alpha1
	ns   string
}

var dependencyproxiesResource = schema.GroupVersionResource{Group: "admiral.io", Version: "v1alpha1", Resource: "dependencyproxies"}

var dependencyproxiesKind = schema.GroupVersionKind{Group: "admiral.io", Version: "v1alpha1", Kind: "DependencyProxy"}

// Get takes name of the dependencyProxy, and returns the corresponding dependencyProxy object, and an error if there is any.
func (c *FakeDependencyProxies) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.DependencyProxy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(dependencyproxiesResource, c.ns, name), &v1alpha1.DependencyProxy{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.DependencyProxy), err
}

// List takes label and field selectors, and returns the list of DependencyProxies that match those selectors.
func (c *FakeDependencyProxies) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.DependencyProxyList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(dependencyproxiesResource, dependencyproxiesKind, c.ns, opts), &v1alpha1.DependencyProxyList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.DependencyProxyList{ListMeta: obj.(*v1alpha1.DependencyProxyList).ListMeta}
	for _, item := range obj.(*v1alpha1.DependencyProxyList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested dependencyProxies.
func (c *FakeDependencyProxies) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(dependencyproxiesResource, c.ns, opts))

}

// Create takes the representation of a dependencyProxy and creates it.  Returns the server's representation of the dependencyProxy, and an error, if there is any.
func (c *FakeDependencyProxies) Create(ctx context.Context, dependencyProxy *v1alpha1.DependencyProxy, opts v1.CreateOptions) (result *v1alpha1.DependencyProxy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(dependencyproxiesResource, c.ns, dependencyProxy), &v1alpha1.DependencyProxy{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.DependencyProxy), err
}

// Update takes the representation of a dependencyProxy and updates it. Returns the server's representation of the dependencyProxy, and an error, if there is any.
func (c *FakeDependencyProxies) Update(ctx context.Context, dependencyProxy *v1alpha1.DependencyProxy, opts v1.UpdateOptions) (result *v1alpha1.DependencyProxy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(dependencyproxiesResource, c.ns, dependencyProxy), &v1alpha1.DependencyProxy{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.DependencyProxy), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeDependencyProxies) UpdateStatus(ctx context.Context, dependencyProxy *v1alpha1.DependencyProxy, opts v1.UpdateOptions) (*v1alpha1.DependencyProxy, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(dependencyproxiesResource, "status", c.ns, dependencyProxy), &v1alpha1.DependencyProxy{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.DependencyProxy), err
}

// Delete takes name of the dependencyProxy and deletes it. Returns an error if one occurs.
func (c *FakeDependencyProxies) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(dependencyproxiesResource, c.ns, name, opts), &v1alpha1.DependencyProxy{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeDependencyProxies) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(dependencyproxiesResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.DependencyProxyList{})
	return err
}

// Patch applies the patch and returns the patched dependencyProxy.
func (c *FakeDependencyProxies) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.DependencyProxy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(dependencyproxiesResource, c.ns, name, pt, data, subresources...), &v1alpha1.DependencyProxy{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.DependencyProxy), err
}
