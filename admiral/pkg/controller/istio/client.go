package istio

import (
	"errors"
	"fmt"
	"github.com/gogo/protobuf/proto"
	multierror "github.com/hashicorp/go-multierror"
	"istio.io/istio/galley/pkg/metadata"
	"istio.io/istio/pilot/pkg/model/test"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/wait"             // import GKE cluster authentication plugin
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // import OIDC cluster authentication plugin, e.g. for Tectonic
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"time"

	"istio.io/istio/pilot/pkg/model"
	kubecfg "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/log"
)

// ConfigDescriptor defines the bijection between the short type name and its
// fully qualified protobuf message name
type ConfigDescriptor []ProtoSchema

// ProtoSchema provides description of the configuration schema and its key function
// nolint: maligned
type ProtoSchema struct {
	// ClusterScoped is true for resource in cluster-level.
	ClusterScoped bool

	// Type is the config proto type.
	Type string

	// Plural is the type in plural.
	Plural string

	// Group is the config proto group.
	Group string

	// Version is the config proto version.
	Version string

	// MessageName refers to the protobuf message type name corresponding to the type
	MessageName string

	// Validate configuration as a protobuf message assuming the object is an
	// instance of the expected message type
	Validate func(name, namespace string, config proto.Message) error

	// MCP collection for this configuration resource schema
	Collection string
}

var (
	// MockConfig is used purely for testing
	MockConfigProto = ProtoSchema{
		Type:        "mock-config",
		Plural:      "mock-configs",
		Group:       "test",
		Version:     "v1",
		MessageName: "test.MockConfig",
		Validate: func(name, namespace string, config proto.Message) error {
			if config.(*test.MockConfig).Key == "" {
				return errors.New("empty key")
			}
			return nil
		},
	}

	// VirtualService describes v1alpha3 route rules
	VirtualServiceProto = ProtoSchema{
		Type:        "virtual-service",
		Plural:      "virtual-services",
		Group:       "networking",
		Version:     "v1alpha3",
		MessageName: "istio.networking.v1alpha3.VirtualService",
		Validate:    model.ValidateVirtualService,
		Collection:  metadata.IstioNetworkingV1alpha3Virtualservices.Collection.String(),
	}

	// Gateway describes a gateway (how a proxy is exposed on the network)
	GatewayProto = ProtoSchema{
		Type:        "gateway",
		Plural:      "gateways",
		Group:       "networking",
		Version:     "v1alpha3",
		MessageName: "istio.networking.v1alpha3.Gateway",
		Validate:    model.ValidateGateway,
		Collection:  metadata.IstioNetworkingV1alpha3Gateways.Collection.String(),
	}

	// ServiceEntry describes service entries
	ServiceEntryProto = ProtoSchema{
		Type:        "service-entry",
		Plural:      "service-entries",
		Group:       "networking",
		Version:     "v1alpha3",
		MessageName: "istio.networking.v1alpha3.ServiceEntry",
		Validate:    model.ValidateServiceEntry,
		Collection:  metadata.IstioNetworkingV1alpha3Serviceentries.Collection.String(),
	}

	// DestinationRule describes destination rules
	DestinationRuleProto = ProtoSchema{
		Type:        "destination-rule",
		Plural:      "destination-rules",
		Group:       "networking",
		Version:     "v1alpha3",
		MessageName: "istio.networking.v1alpha3.DestinationRule",
		Validate:    model.ValidateDestinationRule,
		Collection:  metadata.IstioNetworkingV1alpha3Destinationrules.Collection.String(),
	}

	// EnvoyFilter describes additional envoy filters to be inserted by Pilot
	EnvoyFilterProto = ProtoSchema{
		Type:        "envoy-filter",
		Plural:      "envoy-filters",
		Group:       "networking",
		Version:     "v1alpha3",
		MessageName: "istio.networking.v1alpha3.EnvoyFilter",
		Validate:    model.ValidateEnvoyFilter,
		Collection:  metadata.IstioNetworkingV1alpha3Envoyfilters.Collection.String(),
	}

	// Sidecar describes the listeners associated with sidecars in a namespace
	SidecarProto = ProtoSchema{
		Type:        "sidecar",
		Plural:      "sidecars",
		Group:       "networking",
		Version:     "v1alpha3",
		MessageName: "istio.networking.v1alpha3.Sidecar",
		Validate:    model.ValidateSidecar,
		Collection:  metadata.IstioNetworkingV1alpha3Sidecars.Collection.String(),
	}

	// IstioConfigTypes lists Istio config types with schemas and validation
	IstioConfigTypes = ConfigDescriptor{
		VirtualServiceProto,
		GatewayProto,
		ServiceEntryProto,
		DestinationRuleProto,
		EnvoyFilterProto,
		SidecarProto,
	}
)

// GetByType finds a schema by type if it is available
func (descriptor ConfigDescriptor) GetByType(name string) (ProtoSchema, bool) {
	for _, schema := range descriptor {
		if schema.Type == name {
			return schema, true
		}
	}
	return ProtoSchema{}, false
}

// IstioObject is a k8s wrapper interface for config objects
type IstioObject interface {
	runtime.Object
	GetSpec() map[string]interface{}
	SetSpec(map[string]interface{})
	GetObjectMeta() meta_v1.ObjectMeta
	SetObjectMeta(meta_v1.ObjectMeta)
}

// IstioObjectList is a k8s wrapper interface for config lists
type IstioObjectList interface {
	runtime.Object
	GetItems() []IstioObject
}

// Client is a basic REST client for CRDs implementing config store
type Client struct {
	// Map of apiVersion to restClient.
	clientset map[string]*restClient

	// domainSuffix for the config metadata
	domainSuffix string
}

type restClient struct {
	apiVersion schema.GroupVersion

	// descriptor from the same apiVersion.
	descriptor ConfigDescriptor

	// types of the schema and objects in the descriptor.
	types []*schemaType

	// restconfig for REST type descriptors
	restconfig *rest.Config

	// dynamic REST client for accessing config CRDs
	dynamic *rest.RESTClient
}

func apiVersion(schema *ProtoSchema) string {
	return ResourceGroup(schema) + "/" + schema.Version
}

func apiVersionFromConfig(config *Config) string {
	return config.Group + "/" + config.Version
}

func newClientSet(descriptor ConfigDescriptor) (map[string]*restClient, error) {
	cs := make(map[string]*restClient)
	for _, typ := range descriptor {
		s, exists := knownTypes[typ.Type]
		if !exists {
			return nil, fmt.Errorf("missing known type for %q", typ.Type)
		}

		rc, ok := cs[apiVersion(&typ)]
		if !ok {
			// create a new client if one doesn't already exist
			rc = &restClient{
				apiVersion: schema.GroupVersion{
					Group:   ResourceGroup(&typ),
					Version: typ.Version,
				},
			}
			cs[apiVersion(&typ)] = rc
		}
		rc.descriptor = append(rc.descriptor, typ)
		rc.types = append(rc.types, &s)
	}
	return cs, nil
}

func (rc *restClient) init(kubeconfig *rest.Config, context string) error {

	cfg, err := rc.updateRESTConfig(kubeconfig)
	if err != nil {
		return err
	}

	dynamic, err := rest.RESTClientFor(cfg)
	if err != nil {
		return err
	}

	rc.restconfig = kubeconfig
	rc.dynamic = dynamic
	return nil
}

// createRESTConfig for cluster API server, pass empty config file for in-cluster
func (rc *restClient) updateRESTConfig(cfg *rest.Config) (config *rest.Config, err error) {
	config = cfg
	config.GroupVersion = &rc.apiVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON

	types := runtime.NewScheme()
	schemeBuilder := runtime.NewSchemeBuilder(
		func(scheme *runtime.Scheme) error {
			for _, kind := range rc.types {
				scheme.AddKnownTypes(rc.apiVersion, kind.object, kind.collection)
			}
			meta_v1.AddToGroupVersion(scheme, rc.apiVersion)
			return nil
		})
	err = schemeBuilder.AddToScheme(types)
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(types)}

	return
}

// createRESTConfig for cluster API server, pass empty config file for in-cluster
func (rc *restClient) createRESTConfig(kubeconfig string, context string) (config *rest.Config, err error) {
	config, err = kubecfg.BuildClientConfig(kubeconfig, context)

	if err != nil {
		return nil, err
	}

	config.GroupVersion = &rc.apiVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON

	types := runtime.NewScheme()
	schemeBuilder := runtime.NewSchemeBuilder(
		func(scheme *runtime.Scheme) error {
			for _, kind := range rc.types {
				scheme.AddKnownTypes(rc.apiVersion, kind.object, kind.collection)
			}
			meta_v1.AddToGroupVersion(scheme, rc.apiVersion)
			return nil
		})
	err = schemeBuilder.AddToScheme(types)
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(types)}

	return
}

func NewClient(config *rest.Config, context string, descriptor ConfigDescriptor, domainSuffix string) (*Client, error) {
	cs, err := newClientSet(descriptor)
	if err != nil {
		return nil, err
	}

	out := &Client{
		clientset:    cs,
		domainSuffix: domainSuffix,
	}

	for _, v := range out.clientset {
		if err := v.init(config, context); err != nil {
			return nil, err
		}
	}

	return out, nil
}

// RegisterResources sends a request to create CRDs and waits for them to initialize
func (cl *Client) RegisterResources() error {
	for k, rc := range cl.clientset {
		log.Infof("registering for apiVersion %v", k)
		if err := rc.registerResources(); err != nil {
			return err
		}
	}
	return nil
}

func (rc *restClient) registerResources() error {
	cs, err := apiextensionsclient.NewForConfig(rc.restconfig)
	if err != nil {
		return err
	}

	skipCreate := true
	for _, schema := range rc.descriptor {
		name := ResourceName(schema.Plural) + "." + ResourceGroup(&schema)
		crd, errGet := cs.ApiextensionsV1beta1().CustomResourceDefinitions().Get(name, meta_v1.GetOptions{})
		if errGet != nil {
			skipCreate = false
			break // create the resources
		}
		for _, cond := range crd.Status.Conditions {
			if cond.Type == apiextensionsv1beta1.Established &&
				cond.Status == apiextensionsv1beta1.ConditionTrue {
				continue
			}

			if cond.Type == apiextensionsv1beta1.NamesAccepted &&
				cond.Status == apiextensionsv1beta1.ConditionTrue {
				continue
			}

			log.Warnf("Not established: %v", name)
			skipCreate = false
			break
		}
	}

	if skipCreate {
		return nil
	}

	for _, schema := range rc.descriptor {
		g := ResourceGroup(&schema)
		name := ResourceName(schema.Plural) + "." + g
		crdScope := apiextensionsv1beta1.NamespaceScoped
		if schema.ClusterScoped {
			crdScope = apiextensionsv1beta1.ClusterScoped
		}
		crd := &apiextensionsv1beta1.CustomResourceDefinition{
			ObjectMeta: meta_v1.ObjectMeta{
				Name: name,
			},
			Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
				Group:   g,
				Version: schema.Version,
				Scope:   crdScope,
				Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
					Plural: ResourceName(schema.Plural),
					Kind:   KebabCaseToCamelCase(schema.Type),
				},
			},
		}
		log.Infof("registering CRD %q", name)
		_, err = cs.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}

	// wait for CRD being established
	errPoll := wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
	descriptor:
		for _, schema := range rc.descriptor {
			name := ResourceName(schema.Plural) + "." + ResourceGroup(&schema)
			crd, errGet := cs.ApiextensionsV1beta1().CustomResourceDefinitions().Get(name, meta_v1.GetOptions{})
			if errGet != nil {
				return false, errGet
			}
			for _, cond := range crd.Status.Conditions {
				switch cond.Type {
				case apiextensionsv1beta1.Established:
					if cond.Status == apiextensionsv1beta1.ConditionTrue {
						log.Infof("established CRD %q", name)
						continue descriptor
					}
				case apiextensionsv1beta1.NamesAccepted:
					if cond.Status == apiextensionsv1beta1.ConditionFalse {
						log.Warnf("name conflict: %v", cond.Reason)
					}
				}
			}
			log.Infof("missing status condition for %q", name)
			return false, nil
		}
		return true, nil
	})

	if errPoll != nil {
		log.Error("failed to verify CRD creation")
		return errPoll
	}

	return nil
}

// DeregisterResources removes third party resources
func (cl *Client) DeregisterResources() error {
	for k, rc := range cl.clientset {
		log.Infof("deregistering for apiVersion %s", k)
		if err := rc.deregisterResources(); err != nil {
			return err
		}
	}
	return nil
}

func (rc *restClient) deregisterResources() error {
	cs, err := apiextensionsclient.NewForConfig(rc.restconfig)
	if err != nil {
		return err
	}

	var errs error
	for _, schema := range rc.descriptor {
		name := ResourceName(schema.Plural) + "." + ResourceGroup(&schema)
		err := cs.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(name, nil)
		errs = multierror.Append(errs, err)
	}
	return errs
}

// ConfigDescriptor for the store
func (cl *Client) ConfigDescriptor() ConfigDescriptor {
	d := make(ConfigDescriptor, 0, len(cl.clientset))
	for _, rc := range cl.clientset {
		d = append(d, rc.descriptor...)
	}
	return d
}

// Get implements store interface
func (cl *Client) Get(typ, name, namespace string) *Config {
	s, ok := knownTypes[typ]
	if !ok {
		log.Warn("unknown type " + typ)
		return nil
	}
	rc, ok := cl.clientset[apiVersion(&s.schema)]
	if !ok {
		log.Warn("cannot find client for type " + typ)
		return nil
	}

	schema, exists := rc.descriptor.GetByType(typ)
	if !exists {
		log.Warn("cannot find proto schema for type " + typ)
		return nil
	}

	config := s.object.DeepCopyObject().(IstioObject)
	err := rc.dynamic.Get().
		Namespace(namespace).
		Resource(ResourceName(schema.Plural)).
		Name(name).
		Do().Into(config)

	if err != nil {
		log.Warna(err)
		return nil
	}

	out, err := ConvertObject(schema, config, cl.domainSuffix)
	if err != nil {
		log.Warna(err)
		return nil
	}
	return out
}

// Create implements store interface
func (cl *Client) Create(config Config) (string, error) {
	rc, ok := cl.clientset[apiVersionFromConfig(&config)]
	if !ok {
		return "", fmt.Errorf("unrecognized apiVersion %q", config)
	}

	schema, exists := rc.descriptor.GetByType(config.Type)
	if !exists {
		return "", fmt.Errorf("unrecognized type %q", config.Type)
	}

	if err := schema.Validate(config.Name, config.Namespace, config.Spec); err != nil {
		return "", multierror.Prefix(err, "validation error:")
	}

	out, err := ConvertConfig(schema, config)
	if err != nil {
		return "", err
	}

	obj := knownTypes[schema.Type].object.DeepCopyObject().(IstioObject)
	err = rc.dynamic.Post().
		Namespace(out.GetObjectMeta().Namespace).
		Resource(ResourceName(schema.Plural)).
		Body(out).
		Do().Into(obj)
	if err != nil {
		return "", err
	}

	return obj.GetObjectMeta().ResourceVersion, nil
}

// Update implements store interface
func (cl *Client) Update(config Config) (string, error) {
	rc, ok := cl.clientset[apiVersionFromConfig(&config)]
	if !ok {
		return "", fmt.Errorf("unrecognized apiVersion %q", config)
	}
	schema, exists := rc.descriptor.GetByType(config.Type)
	if !exists {
		return "", fmt.Errorf("unrecognized type %q", config.Type)
	}

	if err := schema.Validate(config.Name, config.Namespace, config.Spec); err != nil {
		return "", multierror.Prefix(err, "validation error:")
	}

	if config.ResourceVersion == "" {
		return "", fmt.Errorf("revision is required")
	}

	out, err := ConvertConfig(schema, config)
	if err != nil {
		return "", err
	}

	obj := knownTypes[schema.Type].object.DeepCopyObject().(IstioObject)
	err = rc.dynamic.Put().
		Namespace(out.GetObjectMeta().Namespace).
		Resource(ResourceName(schema.Plural)).
		Name(out.GetObjectMeta().Name).
		Body(out).
		Do().Into(obj)
	if err != nil {
		return "", err
	}

	return obj.GetObjectMeta().ResourceVersion, nil
}

// Delete implements store interface
func (cl *Client) Delete(typ, name, namespace string) error {
	s, ok := knownTypes[typ]
	if !ok {
		return fmt.Errorf("unrecognized type %q", typ)
	}
	rc, ok := cl.clientset[apiVersion(&s.schema)]
	if !ok {
		return fmt.Errorf("unrecognized apiVersion %v", s.schema)
	}
	schema, exists := rc.descriptor.GetByType(typ)
	if !exists {
		return fmt.Errorf("missing type %q", typ)
	}

	return rc.dynamic.Delete().
		Namespace(namespace).
		Resource(ResourceName(schema.Plural)).
		Name(name).
		Do().Error()
}

// List implements store interface
func (cl *Client) List(typ, namespace string) ([]Config, error) {
	s, ok := knownTypes[typ]
	if !ok {
		return nil, fmt.Errorf("unrecognized type %q", typ)
	}
	rc, ok := cl.clientset[apiVersion(&s.schema)]
	if !ok {
		return nil, fmt.Errorf("unrecognized apiVersion %v", s.schema)
	}
	schema, exists := rc.descriptor.GetByType(typ)
	if !exists {
		return nil, fmt.Errorf("missing type %q", typ)
	}

	list := knownTypes[schema.Type].collection.DeepCopyObject().(IstioObjectList)
	errs := rc.dynamic.Get().
		Namespace(namespace).
		Resource(ResourceName(schema.Plural)).
		Do().Into(list)

	out := make([]Config, 0)
	for _, item := range list.GetItems() {
		obj, err := ConvertObject(schema, item, cl.domainSuffix)
		if err != nil {
			errs = multierror.Append(errs, err)
		} else {
			out = append(out, *obj)
		}
	}
	return out, errs
}
