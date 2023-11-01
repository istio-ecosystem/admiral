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

package v1

import (
	"net/http"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned/scheme"
	rest "k8s.io/client-go/rest"
)

type AdmiralV1Interface interface {
	RESTClient() rest.Interface
	DependenciesGetter
	DependencyProxiesGetter
	GlobalTrafficPoliciesGetter
	OutlierDetectionsGetter
	RoutingPoliciesGetter
}

// AdmiralV1Client is used to interact with features provided by the admiral.io group.
type AdmiralV1Client struct {
	restClient rest.Interface
}

func (c *AdmiralV1Client) Dependencies(namespace string) DependencyInterface {
	return newDependencies(c, namespace)
}

func (c *AdmiralV1Client) DependencyProxies(namespace string) DependencyProxyInterface {
	return newDependencyProxies(c, namespace)
}

func (c *AdmiralV1Client) GlobalTrafficPolicies(namespace string) GlobalTrafficPolicyInterface {
	return newGlobalTrafficPolicies(c, namespace)
}

func (c *AdmiralV1Client) OutlierDetections(namespace string) OutlierDetectionInterface {
	return newOutlierDetections(c, namespace)
}

func (c *AdmiralV1Client) RoutingPolicies(namespace string) RoutingPolicyInterface {
	return newRoutingPolicies(c, namespace)
}

// NewForConfig creates a new AdmiralV1Client for the given config.
// NewForConfig is equivalent to NewForConfigAndClient(c, httpClient),
// where httpClient was generated with rest.HTTPClientFor(c).
func NewForConfig(c *rest.Config) (*AdmiralV1Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	httpClient, err := rest.HTTPClientFor(&config)
	if err != nil {
		return nil, err
	}
	return NewForConfigAndClient(&config, httpClient)
}

// NewForConfigAndClient creates a new AdmiralV1Client for the given config and http client.
// Note the http client provided takes precedence over the configured transport values.
func NewForConfigAndClient(c *rest.Config, h *http.Client) (*AdmiralV1Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientForConfigAndClient(&config, h)
	if err != nil {
		return nil, err
	}
	return &AdmiralV1Client{client}, nil
}

// NewForConfigOrDie creates a new AdmiralV1Client for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *AdmiralV1Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new AdmiralV1Client for the given RESTClient.
func New(c rest.Interface) *AdmiralV1Client {
	return &AdmiralV1Client{c}
}

func setConfigDefaults(config *rest.Config) error {
	gv := v1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *AdmiralV1Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}
