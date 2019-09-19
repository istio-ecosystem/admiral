// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package istio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	multierror "github.com/hashicorp/go-multierror"
	yaml2 "gopkg.in/yaml.v2"

	"io"
	"reflect"
	"strings"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kubeyaml "k8s.io/apimachinery/pkg/util/yaml"

	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/log"
)

// ConvertObject converts an IstioObject k8s-style object to the
// internal configuration model.
func ConvertObject(schema ProtoSchema, object IstioObject, domain string) (*Config, error) {
	data, err := schema.FromJSONMap(object.GetSpec())
	if err != nil {
		return nil, err
	}
	meta := object.GetObjectMeta()

	return &Config{
		ConfigMeta: ConfigMeta{
			Type:              schema.Type,
			Group:             ResourceGroup(&schema),
			Version:           schema.Version,
			Name:              meta.Name,
			Namespace:         meta.Namespace,
			Domain:            domain,
			Labels:            meta.Labels,
			Annotations:       meta.Annotations,
			ResourceVersion:   meta.ResourceVersion,
			CreationTimestamp: meta.CreationTimestamp.Time,
		},
		Spec: data,
	}, nil
}

// ConvertObject converts an k8s-style object to the
// internal configuration model.
func ConvertIstioType(schema ProtoSchema, object proto.Message, name string, namespace string) (*Config, error) {

	return &Config{
		ConfigMeta: ConfigMeta{
			Type:              schema.Type,
			Group:             ResourceGroup(&schema),
			Version:           schema.Version,
			Name: name,
			Namespace: namespace,
		},
		Spec: object,
	}, nil
}

// ConvertObjectFromUnstructured converts an IstioObject k8s-style object to the
// internal configuration model.
func ConvertObjectFromUnstructured(schema ProtoSchema, un *unstructured.Unstructured, domain string) (*model.Config, error) {
	data, err := schema.FromJSONMap(un.Object["spec"])
	if err != nil {
		return nil, err
	}

	return &model.Config{
		ConfigMeta: model.ConfigMeta{
			Type:              schema.Type,
			Group:             ResourceGroup(&schema),
			Version:           schema.Version,
			Name:              un.GetName(),
			Namespace:         un.GetNamespace(),
			Domain:            domain,
			Labels:            un.GetLabels(),
			Annotations:       un.GetAnnotations(),
			ResourceVersion:   un.GetResourceVersion(),
			CreationTimestamp: un.GetCreationTimestamp().Time,
		},
		Spec: data,
	}, nil
}

// ConvertConfig translates Istio config to k8s config JSON
func ConvertConfig(schema ProtoSchema, config Config) (IstioObject, error) {
	spec, err := model.ToJSONMap(config.Spec)
	if err != nil {
		return nil, err
	}
	namespace := config.Namespace
	if namespace == "" {
		namespace = meta_v1.NamespaceDefault
	}
	out := knownTypes[schema.Type].object.DeepCopyObject().(IstioObject)
	out.SetObjectMeta(meta_v1.ObjectMeta{
		Name:            config.Name,
		Namespace:       namespace,
		ResourceVersion: config.ResourceVersion,
		Labels:          config.Labels,
		Annotations:     config.Annotations,
	})
	out.SetSpec(spec)

	return out, nil
}

// ResourceName converts "my-name" to "myname".
// This is needed by k8s API server as dashes prevent kubectl from accessing CRDs
func ResourceName(s string) string {
	return strings.Replace(s, "-", "", -1)
}

// ResourceGroup generates the k8s API group for each schema.
func ResourceGroup(schema *ProtoSchema) string {
	return schema.Group + model.IstioAPIGroupDomain
}

// TODO - add special cases for type-to-kind and kind-to-type
// conversions with initial-isms. Consider adding additional type
// information to the abstract model and/or elevating k8s
// representation to first-class type to avoid extra conversions.

// KebabCaseToCamelCase converts "my-name" to "MyName"
func KebabCaseToCamelCase(s string) string {
	switch s {
	case "http-api-spec":
		return "HTTPAPISpec"
	case "http-api-spec-binding":
		return "HTTPAPISpecBinding"
	default:
		words := strings.Split(s, "-")
		out := ""
		for _, word := range words {
			out = out + strings.Title(word)
		}
		return out
	}
}

// CamelCaseToKebabCase converts "MyName" to "my-name"
func CamelCaseToKebabCase(s string) string {
	switch s {
	case "HTTPAPISpec":
		return "http-api-spec"
	case "HTTPAPISpecBinding":
		return "http-api-spec-binding"
	default:
		var out bytes.Buffer
		for i := range s {
			if 'A' <= s[i] && s[i] <= 'Z' {
				if i > 0 {
					out.WriteByte('-')
				}
				out.WriteByte(s[i] - 'A' + 'a')
			} else {
				out.WriteByte(s[i])
			}
		}
		return out.String()
	}
}

func parseInputsImpl(inputs string, withValidate bool) ([]Config, []IstioKind, error) {
	var varr []Config
	var others []IstioKind
	reader := bytes.NewReader([]byte(inputs))
	var empty = IstioKind{}

	// We store configs as a YaML stream; there may be more than one decoder.
	yamlDecoder := kubeyaml.NewYAMLOrJSONDecoder(reader, 512*1024)
	for {
		obj := IstioKind{}
		err := yamlDecoder.Decode(&obj)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("cannot parse proto message: %v", err)
		}
		if reflect.DeepEqual(obj, empty) {
			continue
		}

		schema, exists := IstioConfigTypes.GetByType(CamelCaseToKebabCase(obj.Kind))
		if !exists {
			log.Debugf("unrecognized type %v", obj.Kind)
			others = append(others, obj)
			continue
		}

		config, err := ConvertObject(schema, &obj, "")
		if err != nil {
			return nil, nil, fmt.Errorf("cannot parse proto message: %v", err)
		}

		if withValidate {
			if err := schema.Validate(config.Name, config.Namespace, config.Spec); err != nil {
				return nil, nil, fmt.Errorf("configuration is invalid: %v", err)
			}
		}

		varr = append(varr, *config)
	}

	return varr, others, nil
}

// ParseInputs reads multiple documents from `kubectl` output and checks with
// the schema. It also returns the list of unrecognized kinds as the second
// response.
//
// NOTE: This function only decodes a subset of the complete k8s
// ObjectMeta as identified by the fields in model.ConfigMeta. This
// would typically only be a problem if a user dumps an configuration
// object with kubectl and then re-ingests it.
func ParseInputs(inputs string) ([]Config, []IstioKind, error) {
	return parseInputsImpl(inputs, true)
}

// ParseInputsWithoutValidation same as ParseInputs, but do not apply schema validation.
func ParseInputsWithoutValidation(inputs string) ([]Config, []IstioKind, error) {
	return parseInputsImpl(inputs, false)
}

// Make creates a new instance of the proto message
func (ps *ProtoSchema) Make() (proto.Message, error) {
	pbt := proto.MessageType(ps.MessageName)
	if pbt == nil {
		return nil, fmt.Errorf("unknown type %q", ps.MessageName)
	}
	return reflect.New(pbt.Elem()).Interface().(proto.Message), nil
}

// ToJSON marshals a proto to canonical JSON
func ToJSON(msg proto.Message) (string, error) {
	return ToJSONWithIndent(msg, "")
}

// ToJSONWithIndent marshals a proto to canonical JSON with pretty printed string
func ToJSONWithIndent(msg proto.Message, indent string) (string, error) {
	if msg == nil {
		return "", errors.New("unexpected nil message")
	}

	// Marshal from proto to json bytes
	m := jsonpb.Marshaler{Indent: indent}
	return m.MarshalToString(msg)
}

// ToYAML marshals a proto to canonical YAML
func ToYAML(msg proto.Message) (string, error) {
	js, err := ToJSON(msg)
	if err != nil {
		return "", err
	}
	yml, err := yaml.JSONToYAML([]byte(js))
	return string(yml), err
}

// ToJSONMap converts a proto message to a generic map using canonical JSON encoding
// JSON encoding is specified here: https://developers.google.com/protocol-buffers/docs/proto3#json
func ToJSONMap(msg proto.Message) (map[string]interface{}, error) {
	js, err := ToJSON(msg)
	if err != nil {
		return nil, err
	}

	// Unmarshal from json bytes to go map
	var data map[string]interface{}
	err = json.Unmarshal([]byte(js), &data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// FromJSON converts a canonical JSON to a proto message
func (ps *ProtoSchema) FromJSON(js string) (proto.Message, error) {
	pb, err := ps.Make()
	if err != nil {
		return nil, err
	}
	if err = ApplyJSON(js, pb, true); err != nil {
		return nil, err
	}
	return pb, nil
}

// ApplyJSON unmarshals a JSON string into a proto message. Unknown fields will produce an
//// error unless strict is set to false.
func ApplyJSON(js string, pb proto.Message, strict bool) error {
	reader := strings.NewReader(js)
	m := jsonpb.Unmarshaler{}
	if err := m.Unmarshal(reader, pb); err != nil {
		if strict {
			return err
		}

		log.Warnf("Failed to decode proto: %q. Trying decode with AllowUnknownFields=true", err)
		m.AllowUnknownFields = true
		reader.Reset(js)
		return m.Unmarshal(reader, pb)
	}
	return nil
}

// FromYAML converts a canonical YAML to a proto message
func (ps *ProtoSchema) FromYAML(yml string) (proto.Message, error) {
	pb, err := ps.Make()
	if err != nil {
		return nil, err
	}
	if err = ApplyYAML(yml, pb, true); err != nil {
		return nil, err
	}
	return pb, nil
}

// ApplyYAML unmarshals a YAML string into a proto message. Unknown fields will produce an
// error unless strict is set to false.
func ApplyYAML(yml string, pb proto.Message, strict bool) error {
	js, err := yaml.YAMLToJSON([]byte(yml))
	if err != nil {
		return err
	}
	return ApplyJSON(string(js), pb, strict)
}

// FromJSONMap converts from a generic map to a proto message using canonical JSON encoding
// JSON encoding is specified here: https://developers.google.com/protocol-buffers/docs/proto3#json
func (ps *ProtoSchema) FromJSONMap(data interface{}) (proto.Message, error) {
	// Marshal to YAML bytes
	str, err := yaml2.Marshal(data)
	if err != nil {
		return nil, err
	}
	out, err := ps.FromYAML(string(str))
	if err != nil {
		return nil, multierror.Prefix(err, fmt.Sprintf("YAML decoding error: %v", string(str)))
	}
	return out, nil
}
