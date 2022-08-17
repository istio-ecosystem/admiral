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

	admiralv1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeDependencies implements DependencyInterface
type FakeDependencies struct {
	Fake *FakeAdmiralV1
	ns   string
}

var dependenciesResource = schema.GroupVersionResource{Group: "admiral.io", Version: "v1", Resource: "dependencies"}

var dependenciesKind = schema.GroupVersionKind{Group: "admiral.io", Version: "v1", Kind: "Dependency"}

// Get takes name of the dependency, and returns the corresponding dependency object, and an error if there is any.
func (c *FakeDependencies) Get(ctx context.Context, name string, options v1.GetOptions) (result *admiralv1.Dependency, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(dependenciesResource, c.ns, name), &admiralv1.Dependency{})

	if obj == nil {
		return nil, err
	}
	return obj.(*admiralv1.Dependency), err
}

// List takes label and field selectors, and returns the list of Dependencies that match those selectors.
func (c *FakeDependencies) List(ctx context.Context, opts v1.ListOptions) (result *admiralv1.DependencyList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(dependenciesResource, dependenciesKind, c.ns, opts), &admiralv1.DependencyList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &admiralv1.DependencyList{ListMeta: obj.(*admiralv1.DependencyList).ListMeta}
	for _, item := range obj.(*admiralv1.DependencyList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested dependencies.
func (c *FakeDependencies) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(dependenciesResource, c.ns, opts))

}

// Create takes the representation of a dependency and creates it.  Returns the server's representation of the dependency, and an error, if there is any.
func (c *FakeDependencies) Create(ctx context.Context, dependency *admiralv1.Dependency, opts v1.CreateOptions) (result *admiralv1.Dependency, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(dependenciesResource, c.ns, dependency), &admiralv1.Dependency{})

	if obj == nil {
		return nil, err
	}
	return obj.(*admiralv1.Dependency), err
}

// Update takes the representation of a dependency and updates it. Returns the server's representation of the dependency, and an error, if there is any.
func (c *FakeDependencies) Update(ctx context.Context, dependency *admiralv1.Dependency, opts v1.UpdateOptions) (result *admiralv1.Dependency, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(dependenciesResource, c.ns, dependency), &admiralv1.Dependency{})

	if obj == nil {
		return nil, err
	}
	return obj.(*admiralv1.Dependency), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeDependencies) UpdateStatus(ctx context.Context, dependency *admiralv1.Dependency, opts v1.UpdateOptions) (*admiralv1.Dependency, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(dependenciesResource, "status", c.ns, dependency), &admiralv1.Dependency{})

	if obj == nil {
		return nil, err
	}
	return obj.(*admiralv1.Dependency), err
}

// Delete takes name of the dependency and deletes it. Returns an error if one occurs.
func (c *FakeDependencies) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(dependenciesResource, c.ns, name, opts), &admiralv1.Dependency{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeDependencies) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(dependenciesResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &admiralv1.DependencyList{})
	return err
}

// Patch applies the patch and returns the patched dependency.
func (c *FakeDependencies) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *admiralv1.Dependency, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(dependenciesResource, c.ns, name, pt, data, subresources...), &admiralv1.Dependency{})

	if obj == nil {
		return nil, err
	}
	return obj.(*admiralv1.Dependency), err
}
