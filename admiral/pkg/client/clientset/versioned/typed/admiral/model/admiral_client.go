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

package model

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned/scheme"
	"k8s.io/client-go/rest"
)

type AdmiralModelInterface interface {
	RESTClient() rest.Interface
}

// AdmiralModelClient is used to interact with features provided by the admiral.io group.
type AdmiralModelClient struct {
	restClient rest.Interface
}

// NewForConfig creates a new AdmiralModelClient for the given config.
func NewForConfig(c *rest.Config) (*AdmiralModelClient, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &AdmiralModelClient{client}, nil
}

// NewForConfigOrDie creates a new AdmiralModelClient for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *AdmiralModelClient {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new AdmiralModelClient for the given RESTClient.
func New(c rest.Interface) *AdmiralModelClient {
	return &AdmiralModelClient{c}
}

func setConfigDefaults(config *rest.Config) error {
	gv := model.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *AdmiralModelClient) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}
