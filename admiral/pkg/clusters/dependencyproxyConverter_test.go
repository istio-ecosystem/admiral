package clusters

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidate(t *testing.T) {

	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{},
	}
	admiralParams.LabelSet.EnvKey = "admiral.io/env"

	common.ResetSync()
	common.InitializeConfig(admiralParams)

	testcases := []struct {
		name               string
		dependencyProxyObj *v1.DependencyProxy
		expectedError      error
	}{
		{
			name:               "Given a validating dependency proxy object, when passed dependency proxy obj is nil, the func should return an error",
			dependencyProxyObj: nil,
			expectedError:      fmt.Errorf("dependencyProxyObj is nil"),
		},
		{
			name: "Given a validating dependency proxy object, when passed dependency proxy obj missing annotation, the func should return an error",
			dependencyProxyObj: &v1.DependencyProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			expectedError: fmt.Errorf("dependencyProxyObj.ObjectMeta.Annotations is nil"),
		},
		{
			name: "Given a validating dependency proxy object, when passed dependency proxy obj missing admiral.io/env annotation, the func should return an error",
			dependencyProxyObj: &v1.DependencyProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"foo": "bar",
					},
				},
			},
			expectedError: fmt.Errorf("admiral.io/env is empty"),
		},
		{
			name: "Given a validating dependency proxy object, when passed dependency proxy obj missing proxy config, the func should return an error",
			dependencyProxyObj: &v1.DependencyProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"admiral.io/env": "stage",
					},
				},
				Spec: model.DependencyProxy{},
			},
			expectedError: fmt.Errorf("dependencyProxyObj.Spec.Proxy is nil"),
		},
		{
			name: "Given a validating dependency proxy object, when passed dependency proxy obj missing proxy identity, the func should return an error",
			dependencyProxyObj: &v1.DependencyProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"admiral.io/env": "stage",
					},
				},
				Spec: model.DependencyProxy{
					Proxy: &model.Proxy{
						Identity: "",
					},
				},
			},
			expectedError: fmt.Errorf("dependencyProxyObj.Spec.Proxy.Identity is empty"),
		},
		{
			name: "Given a validating dependency proxy object, when passed dependency proxy obj missing destination config, the func should return an error",
			dependencyProxyObj: &v1.DependencyProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"admiral.io/env": "stage",
					},
				},
				Spec: model.DependencyProxy{
					Proxy: &model.Proxy{
						Identity: "testproxy",
					},
				},
			},
			expectedError: fmt.Errorf("dependencyProxyObj.Spec.Destination is nil"),
		},
		{
			name: "Given a validating dependency proxy object, when passed dependency proxy obj missing destination identity missing, the func should return an error",
			dependencyProxyObj: &v1.DependencyProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"admiral.io/env": "stage",
					},
				},
				Spec: model.DependencyProxy{
					Proxy: &model.Proxy{
						Identity: "testproxy",
					},
					Destination: &model.Destination{
						Identity: "",
					},
				},
			},
			expectedError: fmt.Errorf("dependencyProxyObj.Spec.Destination.Identity is empty"),
		},
		{
			name: "Given a validating dependency proxy object, when passed dependency proxy obj missing dns suffix, the func should return an error",
			dependencyProxyObj: &v1.DependencyProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"admiral.io/env": "stage",
					},
				},
				Spec: model.DependencyProxy{
					Proxy: &model.Proxy{
						Identity: "testproxy",
					},
					Destination: &model.Destination{
						Identity:  "testdestination",
						DnsSuffix: "",
					},
				},
			},
			expectedError: fmt.Errorf("dependencyProxyObj.Spec.Destination.DnsSuffix is empty"),
		},
		{
			name: "Given a validating dependency proxy object, when valid dependency proxy obj is passed, the func should not return an error",
			dependencyProxyObj: &v1.DependencyProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"admiral.io/env": "stage",
					},
				},
				Spec: model.DependencyProxy{
					Proxy: &model.Proxy{
						Identity: "testproxy",
					},
					Destination: &model.Destination{
						Identity:    "testdestination",
						DnsSuffix:   "test00",
						DnsPrefixes: []string{},
					},
				},
			},
			expectedError: nil,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(tc.dependencyProxyObj)
			if err != nil && tc.expectedError != nil {
				if !strings.Contains(err.Error(), tc.expectedError.Error()) {
					t.Errorf("expected %s, got %s", tc.expectedError.Error(), err.Error())
				}
			} else if err != tc.expectedError {
				t.Errorf("expected %v, got %v", tc.expectedError, err)
			}
		})
	}

}

func TestGenerateVirtualServiceHostNames(t *testing.T) {

	virtualServiceHostNameGenerator := &virtualServiceHostNameGenerator{}

	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{},
	}
	admiralParams.LabelSet.EnvKey = "admiral.io/env"

	common.ResetSync()
	common.InitializeConfig(admiralParams)

	testcases := []struct {
		name               string
		dependencyProxyObj *v1.DependencyProxy
		expectedError      error
		expectedHostNames  []string
	}{
		{
			name:               "Given dependencyproxy obj, when passed dependency proxy obj is nil, then the func should return an error",
			dependencyProxyObj: nil,
			expectedError:      fmt.Errorf("failed to generate virtual service hostnames"),
		},
		{
			name: "Given dependencyproxy obj, when valid dependency proxy obj is passed with no dns prefixes, then the func should not return error",
			dependencyProxyObj: &v1.DependencyProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"admiral.io/env": "stage",
					},
				},
				Spec: model.DependencyProxy{
					Proxy: &model.Proxy{
						Identity: "testproxy",
					},
					Destination: &model.Destination{
						Identity:  "testdestination",
						DnsSuffix: "xyz",
					},
				},
			},
			expectedError:     nil,
			expectedHostNames: []string{"stage.testdestination.xyz"},
		},
		{
			name: "Given dependencyproxy obj, when valid dependency proxy obj is passed, then the func should not return error",
			dependencyProxyObj: &v1.DependencyProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"admiral.io/env": "stage",
					},
				},
				Spec: model.DependencyProxy{
					Proxy: &model.Proxy{
						Identity: "testproxy",
					},
					Destination: &model.Destination{
						Identity:    "testdestination",
						DnsSuffix:   "xyz",
						DnsPrefixes: []string{"test00", "test01"},
					},
				},
			},
			expectedError:     nil,
			expectedHostNames: []string{"stage.testdestination.xyz", "test00.testdestination.xyz", "test01.testdestination.xyz"},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			actualVSHostNames, err := virtualServiceHostNameGenerator.GenerateVirtualServiceHostNames(tc.dependencyProxyObj)
			if err != nil && tc.expectedError != nil {
				if !strings.Contains(err.Error(), tc.expectedError.Error()) {
					t.Errorf("expected %s, got %s", tc.expectedError.Error(), err.Error())
				}
			} else if err != tc.expectedError {
				t.Errorf("expected %v, got %v", tc.expectedError, err)
			}

			if err == nil {
				if !reflect.DeepEqual(actualVSHostNames, tc.expectedHostNames) {
					t.Errorf("expected %v, got %v", tc.expectedHostNames, actualVSHostNames)
				}
			}
		})
	}

}

func TestGenerateProxyDestinationHostName(t *testing.T) {

	virtualServiceDestinationHostHostGenerator := &virtualServiceDestinationHostHostGenerator{}

	admiralParams := common.AdmiralParams{
		LabelSet:       &common.LabelSet{},
		HostnameSuffix: "global",
	}
	admiralParams.LabelSet.EnvKey = "admiral.io/env"

	common.ResetSync()
	common.InitializeConfig(admiralParams)

	testcases := []struct {
		name                 string
		dependencyProxyObj   *v1.DependencyProxy
		expectedError        error
		expectedDestHostName string
	}{
		{
			name:               "Given dependencyproxy obj, when passed dependency proxy obj is nil, then the func should return an error",
			dependencyProxyObj: nil,
			expectedError:      fmt.Errorf("failed to generate virtual service destination hostname"),
		},
		{
			name: "Given dependencyproxy obj, when valid dependency proxy obj is passed with no dns prefixes, then the func should not return error",
			dependencyProxyObj: &v1.DependencyProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"admiral.io/env": "stage",
					},
				},
				Spec: model.DependencyProxy{
					Proxy: &model.Proxy{
						Identity: "testproxy",
					},
					Destination: &model.Destination{
						Identity:  "testdestination",
						DnsSuffix: "xyz",
					},
				},
			},
			expectedError:        nil,
			expectedDestHostName: "stage.testproxy.global",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			actualDestHostName, err := virtualServiceDestinationHostHostGenerator.GenerateProxyDestinationHostName(tc.dependencyProxyObj)
			if err != nil && tc.expectedError != nil {
				if !strings.Contains(err.Error(), tc.expectedError.Error()) {
					t.Errorf("expected %s, got %s", tc.expectedError.Error(), err.Error())
				}
			} else if err != tc.expectedError {
				t.Errorf("expected %v, got %v", tc.expectedError, err)
			}

			if err == nil {
				if actualDestHostName != tc.expectedDestHostName {
					t.Errorf("expected %v, got %v", tc.expectedDestHostName, actualDestHostName)
				}
			}
		})
	}

}
