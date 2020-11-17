package v1

import (
	"encoding/json"
	"errors"
	proto "github.com/gogo/protobuf/proto"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"istio.io/api/networking/v1alpha3"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
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

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type GlobalTrafficPolicyList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`

	Items []GlobalTrafficPolicy `json:"items"`
}

//generic cdr object to wrap the ServiceClient api
// +genclient
type ServiceClient struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata"`
	Spec               ClientSpec          `json:"spec"`
	Status             ClientServiceStatus `json:"status"`
}

type ClientSpec struct {
	Selector map[string]string `json:"selector"`
	Http     []ClientHttpRoute `json:"http"`
}

type ClientHttpRoute struct {
	Match   []*v1alpha3.HTTPMatchRequest `json:"match"`
	Timeout *Duration                    `json:"timeout,omitempty"`
	Retries *v1alpha3.HTTPRetry          `json:"retries"`
	//OutlierDetection *v1alpha3.OutlierDetection   `json:"outlierDetection"`
	Fault *v1alpha3.HTTPFaultInjection `json:"fault"`
}

type ClientServiceStatus struct {
	ClusterSynced int32  `json:"clustersSynced"`
	State         string `json:"state"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ServiceClientList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`

	Items []*ServiceClient `json:"items"`
}

func (in *ClientHttpRoute) DeepCopyInto(out *ClientHttpRoute) {
	*out = *in
	if in.Match != nil {
		for _, m := range in.Match {
			out.Match = append(out.Match, proto.Clone(m).(*v1alpha3.HTTPMatchRequest))
		}
	}
	if in.Timeout != nil {
		out.Timeout.Duration = in.Timeout.Duration
	}
	if in.Retries != nil {
		out.Retries = proto.Clone(in.Retries).(*v1alpha3.HTTPRetry)
	}
	//	if in.OutlierDetection != nil {
	//		out.OutlierDetection = proto.Clone(in.OutlierDetection).(*v1alpha3.OutlierDetection)
	//	}
	if in.Fault != nil {
		out.Fault = proto.Clone(in.Fault).(*v1alpha3.HTTPFaultInjection)
	}
	return
}

func (in *ClientHttpRoute) DeepCopy() *ClientHttpRoute {
	if in == nil {
		return nil
	}
	out := new(ClientHttpRoute)
	in.DeepCopyInto(out)
	return out
}

type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		d.Duration = time.Duration(value)
		return nil
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
		return nil
	default:
		return errors.New("invalid duration")
	}
}
