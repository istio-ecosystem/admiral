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

// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1alpha1

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TrafficConfigStatusApplyConfiguration represents an declarative configuration of the TrafficConfigStatus type for use
// with apply.
type TrafficConfigStatusApplyConfiguration struct {
	Message                  *string  `json:"message,omitempty"`
	LastAppliedConfigVersion *string  `json:"lastAppliedConfigVersion,omitempty"`
	LastUpdateTime           *v1.Time `json:"lastUpdateTime,omitempty"`
	Status                   *bool    `json:"status,omitempty"`
}

// TrafficConfigStatusApplyConfiguration constructs an declarative configuration of the TrafficConfigStatus type for use with
// apply.
func TrafficConfigStatus() *TrafficConfigStatusApplyConfiguration {
	return &TrafficConfigStatusApplyConfiguration{}
}

// WithMessage sets the Message field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Message field is set to the value of the last call.
func (b *TrafficConfigStatusApplyConfiguration) WithMessage(value string) *TrafficConfigStatusApplyConfiguration {
	b.Message = &value
	return b
}

// WithLastAppliedConfigVersion sets the LastAppliedConfigVersion field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the LastAppliedConfigVersion field is set to the value of the last call.
func (b *TrafficConfigStatusApplyConfiguration) WithLastAppliedConfigVersion(value string) *TrafficConfigStatusApplyConfiguration {
	b.LastAppliedConfigVersion = &value
	return b
}

// WithLastUpdateTime sets the LastUpdateTime field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the LastUpdateTime field is set to the value of the last call.
func (b *TrafficConfigStatusApplyConfiguration) WithLastUpdateTime(value v1.Time) *TrafficConfigStatusApplyConfiguration {
	b.LastUpdateTime = &value
	return b
}

// WithStatus sets the Status field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Status field is set to the value of the last call.
func (b *TrafficConfigStatusApplyConfiguration) WithStatus(value bool) *TrafficConfigStatusApplyConfiguration {
	b.Status = &value
	return b
}
