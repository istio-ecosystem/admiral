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

package v1alpha1

import (
	"context"
	json "encoding/json"
	"fmt"
	"time"

	v1alpha1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	admiralv1alpha1 "github.com/istio-ecosystem/admiral/admiral/pkg/client/applyconfiguration/admiral/v1alpha1"
	scheme "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// ClientConnectionConfigsGetter has a method to return a ClientConnectionConfigInterface.
// A group's client should implement this interface.
type ClientConnectionConfigsGetter interface {
	ClientConnectionConfigs(namespace string) ClientConnectionConfigInterface
}

// ClientConnectionConfigInterface has methods to work with ClientConnectionConfig resources.
type ClientConnectionConfigInterface interface {
	Create(ctx context.Context, clientConnectionConfig *v1alpha1.ClientConnectionConfig, opts v1.CreateOptions) (*v1alpha1.ClientConnectionConfig, error)
	Update(ctx context.Context, clientConnectionConfig *v1alpha1.ClientConnectionConfig, opts v1.UpdateOptions) (*v1alpha1.ClientConnectionConfig, error)
	UpdateStatus(ctx context.Context, clientConnectionConfig *v1alpha1.ClientConnectionConfig, opts v1.UpdateOptions) (*v1alpha1.ClientConnectionConfig, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.ClientConnectionConfig, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.ClientConnectionConfigList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ClientConnectionConfig, err error)
	Apply(ctx context.Context, clientConnectionConfig *admiralv1alpha1.ClientConnectionConfigApplyConfiguration, opts v1.ApplyOptions) (result *v1alpha1.ClientConnectionConfig, err error)
	ApplyStatus(ctx context.Context, clientConnectionConfig *admiralv1alpha1.ClientConnectionConfigApplyConfiguration, opts v1.ApplyOptions) (result *v1alpha1.ClientConnectionConfig, err error)
	ClientConnectionConfigExpansion
}

// clientConnectionConfigs implements ClientConnectionConfigInterface
type clientConnectionConfigs struct {
	client rest.Interface
	ns     string
}

// newClientConnectionConfigs returns a ClientConnectionConfigs
func newClientConnectionConfigs(c *AdmiralV1alpha1Client, namespace string) *clientConnectionConfigs {
	return &clientConnectionConfigs{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the clientConnectionConfig, and returns the corresponding clientConnectionConfig object, and an error if there is any.
func (c *clientConnectionConfigs) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.ClientConnectionConfig, err error) {
	result = &v1alpha1.ClientConnectionConfig{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("clientconnectionconfigs").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ClientConnectionConfigs that match those selectors.
func (c *clientConnectionConfigs) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.ClientConnectionConfigList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.ClientConnectionConfigList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("clientconnectionconfigs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested clientConnectionConfigs.
func (c *clientConnectionConfigs) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("clientconnectionconfigs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a clientConnectionConfig and creates it.  Returns the server's representation of the clientConnectionConfig, and an error, if there is any.
func (c *clientConnectionConfigs) Create(ctx context.Context, clientConnectionConfig *v1alpha1.ClientConnectionConfig, opts v1.CreateOptions) (result *v1alpha1.ClientConnectionConfig, err error) {
	result = &v1alpha1.ClientConnectionConfig{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("clientconnectionconfigs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(clientConnectionConfig).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a clientConnectionConfig and updates it. Returns the server's representation of the clientConnectionConfig, and an error, if there is any.
func (c *clientConnectionConfigs) Update(ctx context.Context, clientConnectionConfig *v1alpha1.ClientConnectionConfig, opts v1.UpdateOptions) (result *v1alpha1.ClientConnectionConfig, err error) {
	result = &v1alpha1.ClientConnectionConfig{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("clientconnectionconfigs").
		Name(clientConnectionConfig.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(clientConnectionConfig).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *clientConnectionConfigs) UpdateStatus(ctx context.Context, clientConnectionConfig *v1alpha1.ClientConnectionConfig, opts v1.UpdateOptions) (result *v1alpha1.ClientConnectionConfig, err error) {
	result = &v1alpha1.ClientConnectionConfig{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("clientconnectionconfigs").
		Name(clientConnectionConfig.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(clientConnectionConfig).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the clientConnectionConfig and deletes it. Returns an error if one occurs.
func (c *clientConnectionConfigs) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("clientconnectionconfigs").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *clientConnectionConfigs) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("clientconnectionconfigs").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched clientConnectionConfig.
func (c *clientConnectionConfigs) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ClientConnectionConfig, err error) {
	result = &v1alpha1.ClientConnectionConfig{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("clientconnectionconfigs").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}

// Apply takes the given apply declarative configuration, applies it and returns the applied clientConnectionConfig.
func (c *clientConnectionConfigs) Apply(ctx context.Context, clientConnectionConfig *admiralv1alpha1.ClientConnectionConfigApplyConfiguration, opts v1.ApplyOptions) (result *v1alpha1.ClientConnectionConfig, err error) {
	if clientConnectionConfig == nil {
		return nil, fmt.Errorf("clientConnectionConfig provided to Apply must not be nil")
	}
	patchOpts := opts.ToPatchOptions()
	data, err := json.Marshal(clientConnectionConfig)
	if err != nil {
		return nil, err
	}
	name := clientConnectionConfig.Name
	if name == nil {
		return nil, fmt.Errorf("clientConnectionConfig.Name must be provided to Apply")
	}
	result = &v1alpha1.ClientConnectionConfig{}
	err = c.client.Patch(types.ApplyPatchType).
		Namespace(c.ns).
		Resource("clientconnectionconfigs").
		Name(*name).
		VersionedParams(&patchOpts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}

// ApplyStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating ApplyStatus().
func (c *clientConnectionConfigs) ApplyStatus(ctx context.Context, clientConnectionConfig *admiralv1alpha1.ClientConnectionConfigApplyConfiguration, opts v1.ApplyOptions) (result *v1alpha1.ClientConnectionConfig, err error) {
	if clientConnectionConfig == nil {
		return nil, fmt.Errorf("clientConnectionConfig provided to Apply must not be nil")
	}
	patchOpts := opts.ToPatchOptions()
	data, err := json.Marshal(clientConnectionConfig)
	if err != nil {
		return nil, err
	}

	name := clientConnectionConfig.Name
	if name == nil {
		return nil, fmt.Errorf("clientConnectionConfig.Name must be provided to Apply")
	}

	result = &v1alpha1.ClientConnectionConfig{}
	err = c.client.Patch(types.ApplyPatchType).
		Namespace(c.ns).
		Resource("clientconnectionconfigs").
		Name(*name).
		SubResource("status").
		VersionedParams(&patchOpts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
