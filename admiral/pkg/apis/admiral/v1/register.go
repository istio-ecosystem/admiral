package v1

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is the identifier for the API which includes
// the name of the group and the version of the API
var SchemeGroupVersion = schema.GroupVersion{
	Group:   admiral.GroupName,
	Version: "v1alpha1",
}

// create a SchemeBuilder which uses functions to add types to
// the scheme
var (
	SchemeBuilder      runtime.SchemeBuilder
	localSchemeBuilder = &SchemeBuilder
	AddToScheme        = localSchemeBuilder.AddToScheme
)

func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

func init() {
	// We only register manually written functions here. The registration of the
	// generated functions takes place in the generated files. The separation
	// makes the code compile even when the generated files are missing.
	localSchemeBuilder.Register(addKnownTypes)
}

// addKnownTypes adds our types to the API scheme by registering
// MyResource and MyResourceList
func addKnownTypes(scheme *runtime.Scheme) error {
	//scheme.AddUnversionedTypes(
	//	SchemeGroupVersion,
	//	&Dependency{},
	//	&DependencyList{},
	//	&GlobalTrafficPolicy{},
	//	&GlobalTrafficPolicyList{},
	//)

	scheme.AddKnownTypes(
		SchemeGroupVersion,
		&Dependency{},
		&DependencyList{},
		&GlobalTrafficPolicy{},
		&GlobalTrafficPolicyList{},
		&RoutingPolicy{},
		&RoutingPolicyList{},
	)

	// register the type in the scheme
	meta_v1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
