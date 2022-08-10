package v1

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//generic cdr object to wrap the dependency api
type Dependency struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata"`
	Spec               model.Dependency `json:"spec"`
	Status             DependencyStatus `json:"status"`
}

// FooStatus is the status for a Foo resource
type DependencyStatus struct {
	ClusterSynced int32  `json:"clustersSynced"`
	State         string `json:"state"`
}

// FooList is a list of Foo resources
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type DependencyList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`

	Items []Dependency `json:"items"`
}

//generic cdr object to wrap the GlobalTrafficPolicy api
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type GlobalTrafficPolicy struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata"`
	Spec               model.GlobalTrafficPolicy `json:"spec"`
	Status             GlobalTrafficPolicyStatus `json:"status"`
}

// FooStatus is the status for a Foo resource

type GlobalTrafficPolicyStatus struct {
	ClusterSynced int32  `json:"clustersSynced"`
	State         string `json:"state"`
}

// FooList is a list of Foo resources
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type GlobalTrafficPolicyList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`

	Items []GlobalTrafficPolicy `json:"items"`
}

//generic cdr object to wrap the GlobalTrafficPolicy api
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type RoutingPolicy struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata"`
	Spec               model.RoutingPolicy `json:"spec"`
	Status             RoutingPolicyStatus `json:"status"`
}

// FooStatus is the status for a Foo resource

type RoutingPolicyStatus struct {
	ClusterSynced int32  `json:"clustersSynced"`
	State         string `json:"state"`
}

// FooList is a list of Foo resources
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type RoutingPolicyList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`

	Items []RoutingPolicy `json:"items"`
}
