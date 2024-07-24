package v1alpha1

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// generic cdr object to wrap the dependency api
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

// generic cdr object to wrap the GlobalTrafficPolicy api
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

// generic cdr object to wrap the OutlierDetection api
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OutlierDetection struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata"`
	Spec               model.OutlierDetection `json:"spec"`
	Status             OutlierDetectionStatus `json:"status"`
}

// FooStatus is the status for a Foo resource

type OutlierDetectionStatus struct {
	ClusterSynced int32  `json:"clustersSynced"`
	State         string `json:"state"`
}

// OutlierDetectionList is a list of OutlierDetection resources
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OutlierDetectionList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`

	Items []OutlierDetection `json:"items"`
}

// generic cdr object to wrap the RoutingPolicy api
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

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true
// +kubebuilder:printcolumn:name="Destination",type="string",JSONPath=`.spec.destination.identity`
// +kubebuilder:printcolumn:name="Proxy",type="string",JSONPath=`.spec.proxy.identity`
// +kubebuilder:resource:shortName=dp
type DependencyProxy struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata"`
	Spec               model.DependencyProxy `json:"spec"`
	Status             DependencyProxyStatus `json:"status"`
}

// DependencyProxyStatus is the status for a DependencyProxy resource
type DependencyProxyStatus struct {
	State string `json:"state"`
}

// DependencyProxyList is a list of DependencyProxy resources
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type DependencyProxyList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`

	Items []DependencyProxy `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true
// +kubebuilder:resource:shortName=cc
type ClientConnectionConfig struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata"`
	Spec               ClientConnectionConfigSpec   `json:"spec"`
	Status             ClientConnectionConfigStatus `json:"status"`
}

type ClientConnectionConfigSpec struct {
	ConnectionPool model.ConnectionPool `json:"connectionPool"`
	Tunnel         model.Tunnel         `json:"tunnel"`
}

// ClientConnectionConfigStatus is the status for a ClientConnectionConfig resource
type ClientConnectionConfigStatus struct {
	State string `json:"state"`
}

// ClientConnectionConfigList is a list of ClientConnectionConfig resources
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClientConnectionConfigList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata"`

	Items []ClientConnectionConfig `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// TrafficConfig is the Schema for the TrafficConfigs API
type TrafficConfig struct {
	meta_v1.TypeMeta   `json:",inline"`
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TrafficConfigSpec   `json:"spec,omitempty"`
	Status TrafficConfigStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// TrafficConfigList contains a list of TrafficConfig
type TrafficConfigList struct {
	meta_v1.TypeMeta `json:",inline"`
	meta_v1.ListMeta `json:"metadata,omitempty"`
	Items            []TrafficConfig `json:"items"`
}

// QuotaGroupSpec defines the desired state of QuotaGroup specified by the user
type TrafficConfigSpec struct {
	WorkloadEnv []string     `json:"workloadEnvs"`
	EdgeService *EdgeService `json:"edgeService"`
	QuotaGroup  *QuotaGroup  `json:"quotaGroup"`
}

type EdgeService struct {
	DynamicRouting []*DynamicRouting `json:"dynamicRouting"`
	Filters        []*Filter         `json:"filters"`
	Routes         []*Route          `json:"routes"`
	Targets        []*Target         `json:"targets,omitempty"`
	TargetGroups   []*TargetGroup    `json:"targetGroups,omitempty"`
}

type Target struct {
	Name          string  `json:"name"`
	MeshDNS       *string `json:"meshDNS,omitempty"`
	Port          int     `json:"port"`
	SocketTimeout int     `json:"socketTimeout"`
}

type TargetGroup struct {
	Name         string         `json:"name"`
	Weights      []*Weight      `json:"weights"`
	AppOverrides []*AppOverride `json:"appOverrides,omitempty"`
}

type AppOverride struct {
	AssetAlias string `json:"assetAlias"`

	AssetID string    `json:"assetID"` // assetID is just a UUID string
	Weights []*Weight `json:"weights"`
}

type Weight struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
}

type QuotaGroup struct {
	TotalQuotaGroup []*TotalQuotaGroup `json:"totalQuotaGroups"`
	AppQuotaGroups  []*AppQuotaGroup   `json:"appQuotaGroups,omitempty"`
}

type Route struct {
	Name                 string    `json:"name"`
	Inbound              string    `json:"inbound"`
	Outbound             string    `json:"outbound"`
	FilterSelector       string    `json:"filterSelector"`
	WorkloadEnvSelectors []string  `json:"workloadEnvSelectors"`
	Timeout              int       `json:"timeout"`
	Config               []*Config `json:"config,omitempty"`
}

type Config struct {
	TargetGroupSelector string `json:"targetGroupSelector"`
	TargetSelector      string `json:"targetSelector"`
}

type Filter struct {
	Name    string   `json:"name"`
	Retries Retry    `json:"retries"`
	Options []string `json:"options"`
}

type Retry struct {
	Attempts      int    `json:"attempts"`
	PerTryTimeout string `json:"perTryTimeout"`
}

type DynamicRouting struct {
	Name              string `json:"name"`
	Url               string `json:"url"`
	CacheKeyAlgorithm string `json:"cacheKeyAlgorithm"`
	TtlSec            int    `json:"ttlSec"`
	Local             bool   `json:"local"`
}

type TotalQuotaGroup struct {
	Name                 string               `json:"name"`
	Description          string               `json:"description"`
	Quotas               []*Quota             `json:"quotas"`
	WorkloadEnvSelectors []string             `json:"workloadEnvSelectors"`
	RegionLevelLimit     bool                 `json:"regionLevelLimit"`
	CPULimit             *int                 `json:"cpuLimit,omitempty"`
	MemoryLimit          *int                 `json:"memoryLimit,omitempty"`
	PodLevelThreshold    *int                 `json:"podLevelThreshold"`
	FailureModeBehaviour string               `json:"failureModeBehaviour"`
	AdaptiveConcurrency  *AdaptiveConcurrency `json:"adaptiveConcurrency,omitempty"`
}
type AppQuotaGroup struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Quotas               []*Quota `json:"quotas"`
	AssociatedApps       []string `json:"associatedApps"`
	WorkloadEnvSelectors []string `json:"workloadEnvSelectors"`
}

type AdaptiveConcurrency struct {
	LatencyThreshold          string   `json:"latencyThreshold"`
	SkippedURLs               []string `json:"skippedURLs"`
	SampleAggregatePercentile int      `json:"sampleAggregatePercentile"`
	ConcurrencyUpdateInterval string   `json:"concurrencyUpdateInterval"`
	MinRTTCalInterval         string   `json:"minRTTCalInterval"`
	MinRTTCalJitter           int      `json:"minRTTCalJitter"`
	MinRTTCalRequestCount     int      `json:"minRTTCalRequestCount"`
	MinRTTCalMinConcurrency   int      `json:"minRTTCalMinConcurrency"`
	Enabled                   bool     `json:"enabled"`
}

type Quota struct {
	Name       string    `json:"name"`
	TimePeriod string    `json:"timePeriod"`
	MaxAmount  int       `json:"maxAmount"`
	KeyType    string    `json:"keyType"`
	Algorithm  string    `json:"algorithm"`
	Behaviour  string    `json:"behaviour"`
	Rule       string    `json:"rule"`
	Path       string    `json:"path,omitempty"`
	Methods    []string  `json:"methods,omitempty"`
	Headers    []*Header `json:"headers,omitempty"`
}

type Header struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Condition string `json:"condition"` // EQUALS, PREFIX, CONTAINS, REGEX
}

// TrafficConfigStatus defines the observed state of TrafficConfig
type TrafficConfigStatus struct {
	Message                  string       `json:"message"`
	LastAppliedConfigVersion string       `json:"lastAppliedConfigVersion"`
	LastUpdateTime           meta_v1.Time `json:"lastUpdateTime"`
	Status                   bool         `json:"status"`
}
