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
	admiralv1 "github.com/admiral/admiral/pkg/apis/admiral/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeGlobalTrafficPolicies implements GlobalTrafficPolicyInterface
type FakeGlobalTrafficPolicies struct {
	Fake *FakeAdmiralV1
	ns   string
}

var globaltrafficpoliciesResource = schema.GroupVersionResource{Group: "admiral.io", Version: "v1", Resource: "globaltrafficpolicies"}

var globaltrafficpoliciesKind = schema.GroupVersionKind{Group: "admiral.io", Version: "v1", Kind: "GlobalTrafficPolicy"}

// Get takes name of the globalTrafficPolicy, and returns the corresponding globalTrafficPolicy object, and an error if there is any.
func (c *FakeGlobalTrafficPolicies) Get(name string, options v1.GetOptions) (result *admiralv1.GlobalTrafficPolicy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(globaltrafficpoliciesResource, c.ns, name), &admiralv1.GlobalTrafficPolicy{})

	if obj == nil {
		return nil, err
	}
	return obj.(*admiralv1.GlobalTrafficPolicy), err
}

// List takes label and field selectors, and returns the list of GlobalTrafficPolicies that match those selectors.
func (c *FakeGlobalTrafficPolicies) List(opts v1.ListOptions) (result *admiralv1.GlobalTrafficPolicyList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(globaltrafficpoliciesResource, globaltrafficpoliciesKind, c.ns, opts), &admiralv1.GlobalTrafficPolicyList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &admiralv1.GlobalTrafficPolicyList{ListMeta: obj.(*admiralv1.GlobalTrafficPolicyList).ListMeta}
	for _, item := range obj.(*admiralv1.GlobalTrafficPolicyList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested globalTrafficPolicies.
func (c *FakeGlobalTrafficPolicies) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(globaltrafficpoliciesResource, c.ns, opts))

}

// Create takes the representation of a globalTrafficPolicy and creates it.  Returns the server's representation of the globalTrafficPolicy, and an error, if there is any.
func (c *FakeGlobalTrafficPolicies) Create(globalTrafficPolicy *admiralv1.GlobalTrafficPolicy) (result *admiralv1.GlobalTrafficPolicy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(globaltrafficpoliciesResource, c.ns, globalTrafficPolicy), &admiralv1.GlobalTrafficPolicy{})

	if obj == nil {
		return nil, err
	}
	return obj.(*admiralv1.GlobalTrafficPolicy), err
}

// Update takes the representation of a globalTrafficPolicy and updates it. Returns the server's representation of the globalTrafficPolicy, and an error, if there is any.
func (c *FakeGlobalTrafficPolicies) Update(globalTrafficPolicy *admiralv1.GlobalTrafficPolicy) (result *admiralv1.GlobalTrafficPolicy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(globaltrafficpoliciesResource, c.ns, globalTrafficPolicy), &admiralv1.GlobalTrafficPolicy{})

	if obj == nil {
		return nil, err
	}
	return obj.(*admiralv1.GlobalTrafficPolicy), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeGlobalTrafficPolicies) UpdateStatus(globalTrafficPolicy *admiralv1.GlobalTrafficPolicy) (*admiralv1.GlobalTrafficPolicy, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(globaltrafficpoliciesResource, "status", c.ns, globalTrafficPolicy), &admiralv1.GlobalTrafficPolicy{})

	if obj == nil {
		return nil, err
	}
	return obj.(*admiralv1.GlobalTrafficPolicy), err
}

// Delete takes name of the globalTrafficPolicy and deletes it. Returns an error if one occurs.
func (c *FakeGlobalTrafficPolicies) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(globaltrafficpoliciesResource, c.ns, name), &admiralv1.GlobalTrafficPolicy{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeGlobalTrafficPolicies) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(globaltrafficpoliciesResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &admiralv1.GlobalTrafficPolicyList{})
	return err
}

// Patch applies the patch and returns the patched globalTrafficPolicy.
func (c *FakeGlobalTrafficPolicies) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *admiralv1.GlobalTrafficPolicy, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(globaltrafficpoliciesResource, c.ns, name, data, subresources...), &admiralv1.GlobalTrafficPolicy{})

	if obj == nil {
		return nil, err
	}
	return obj.(*admiralv1.GlobalTrafficPolicy), err
}
