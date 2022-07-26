package client

import (
	"context"
	"reflect"
	"testing"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	admiral "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	admiralv1 "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned/typed/admiral/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

func TestList(t *testing.T) {
	gtpClient := &GlobalTrafficPolicy{}
	testCases := []struct {
		name          string
		clientset     admiral.Interface
		namespace     string
		expectedError string
	}{
		{
			name:          "when nil clientset passed, the func should return an error",
			clientset:     nil,
			namespace:     "testNS",
			expectedError: "*admiral.Clientset is nil",
		},
		{
			name:          "when an empty namespace is passed, the func should return an error",
			clientset:     &mockAdmiralClientSet{},
			namespace:     "",
			expectedError: "namespace parameter is empty",
		},
		{
			name: "when a nil AdmiralV1Client is passed, the func should return an error",
			clientset: &mockAdmiralClientSet{
				nil,
			},
			namespace:     "testNS",
			expectedError: "*admiralv1.AdmiralV1Client is nil",
		},
		{
			name: "valid test, should return a gtps list with 1 gtp",
			clientset: &mockAdmiralClientSet{
				&mockAdmiralV1{
					mockGlobalTrafficPolicies: &mockGlobalTrafficPolicies{
						globalTrafficPolicyList: &v1.GlobalTrafficPolicyList{
							Items: []v1.GlobalTrafficPolicy{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "test",
									},
								},
							},
						},
					},
				},
			},
			namespace:     "testNS",
			expectedError: "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			gtps, err := gtpClient.List(context.TODO(), c.clientset, c.namespace, metav1.ListOptions{})
			if err == nil && c.expectedError != "" {
				t.Errorf("expected err: %s got nil", c.expectedError)
			}
			if err != nil && err.Error() != c.expectedError {
				t.Errorf("expected err: %s got %s", c.expectedError, err.Error())
			}
			if err == nil && gtps == nil {
				t.Error("returned gtp list should not be nil")
			}
			if err == nil && len(gtps.Items) == 0 {
				t.Error("expected gtp list to contain 1 item")
			}
		})
	}

}

func TestGetByEnv(t *testing.T) {
	gtpClient := &GlobalTrafficPolicy{}
	validGTP := v1.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testGTP",
			Annotations: map[string]string{gtpEnvAnnotation: "stage"},
		},
	}
	testCases := []struct {
		name          string
		clientset     admiral.Interface
		namespace     string
		env           string
		expectedError string
		expectedGTP   *v1.GlobalTrafficPolicy
	}{
		{
			name:          "when nil clientset passed, the func should return an error",
			clientset:     nil,
			namespace:     "testNS",
			env:           "stage",
			expectedError: "*admiral.Clientset is nil",
		},
		{
			name:          "when an empty namespace is passed, the func should return an error",
			clientset:     &mockAdmiralClientSet{},
			namespace:     "",
			env:           "stage",
			expectedError: "namespace parameter is empty",
		},
		{
			name: "when a nil AdmiralV1Client is passed, the func should return an error",
			clientset: &mockAdmiralClientSet{
				nil,
			},
			namespace:     "testNS",
			env:           "stage",
			expectedError: "*admiralv1.AdmiralV1Client is nil",
		},
		{
			name: "when an empty env is passed, the func should return an error",
			clientset: &mockAdmiralClientSet{
				&mockAdmiralV1{},
			},
			namespace:     "testNS",
			env:           "",
			expectedError: "env parameter is empty",
		},
		{
			name: "when no gtps match the given env, the func should return an error",
			clientset: &mockAdmiralClientSet{
				&mockAdmiralV1{
					mockGlobalTrafficPolicies: &mockGlobalTrafficPolicies{
						globalTrafficPolicyList: &v1.GlobalTrafficPolicyList{
							Items: []v1.GlobalTrafficPolicy{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name: "test",
									},
								},
							},
						},
					},
				},
			},
			namespace:     "testNS",
			env:           "stage",
			expectedError: "no gtp found with env: stage in namespace: testNS",
		},
		{
			name: "when matching gtp is found based on the passed env param, the gtp should be returned with no errors",
			clientset: &mockAdmiralClientSet{
				&mockAdmiralV1{
					mockGlobalTrafficPolicies: &mockGlobalTrafficPolicies{
						globalTrafficPolicyList: &v1.GlobalTrafficPolicyList{
							Items: []v1.GlobalTrafficPolicy{
								validGTP,
							},
						},
					},
				},
			},
			namespace:     "testNS",
			env:           "stage",
			expectedError: "",
			expectedGTP:   &validGTP,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			gtp, err := gtpClient.GetByEnv(context.TODO(), c.clientset, c.namespace, c.env, metav1.ListOptions{})
			if err == nil && c.expectedError != "" {
				t.Errorf("expected err: %s got nil", c.expectedError)
			}
			if err != nil && err.Error() != c.expectedError {
				t.Errorf("expected err: %s got %s", c.expectedError, err.Error())
			}
			if err == nil && gtp == nil {
				t.Error("returned gtp should not be nil")
			}
			if err == nil && !reflect.DeepEqual(gtp, c.expectedGTP) {
				t.Errorf("expected gtp %v got %v", c.expectedGTP, gtp)
			}
		})
	}

}

func TestCreate(t *testing.T) {
	gtpClient := &GlobalTrafficPolicy{}
	validGTP := &v1.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testGTP",
			Annotations: map[string]string{gtpEnvAnnotation: "stage"},
		},
	}
	testCases := []struct {
		name          string
		clientset     admiral.Interface
		namespace     string
		gtp           *v1.GlobalTrafficPolicy
		expectedError string
	}{
		{
			name:          "when nil clientset passed, the func should return an error",
			clientset:     nil,
			namespace:     "testNS",
			expectedError: "*admiral.Clientset is nil",
		},
		{
			name:          "when an empty namespace is passed, the func should return an error",
			clientset:     &mockAdmiralClientSet{},
			namespace:     "",
			expectedError: "namespace parameter is empty",
		},
		{
			name: "when a nil AdmiralV1Client is passed, the func should return an error",
			clientset: &mockAdmiralClientSet{
				nil,
			},
			namespace:     "testNS",
			expectedError: "*admiralv1.AdmiralV1Client is nil",
		},
		{
			name: "when an empty gtp is passed, the func should return an error",
			clientset: &mockAdmiralClientSet{
				nil,
			},
			namespace:     "testNS",
			expectedError: "*admiralv1.AdmiralV1Client is nil",
		},
		{
			name: "when valid gtp is passed, the func should run successfully without returning any errors",
			clientset: &mockAdmiralClientSet{
				&mockAdmiralV1{
					mockGlobalTrafficPolicies: &mockGlobalTrafficPolicies{
						globalTrafficPolicy: validGTP,
					},
				},
			},
			namespace:     "testNS",
			gtp:           validGTP,
			expectedError: "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			gtp, err := gtpClient.Create(context.TODO(), c.clientset, c.namespace, c.gtp, metav1.CreateOptions{})
			if err == nil && c.expectedError != "" {
				t.Errorf("expected err: %s got nil", c.expectedError)
			}
			if err != nil && err.Error() != c.expectedError {
				t.Errorf("expected err: %s got %s", c.expectedError, err.Error())
			}
			if err == nil && gtp == nil {
				t.Error("returned gtp should not be nil")
			}
		})
	}

}

func TestUpdate(t *testing.T) {
	gtpClient := &GlobalTrafficPolicy{}
	validGTP := &v1.GlobalTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "testGTP",
			Annotations: map[string]string{gtpEnvAnnotation: "stage"},
		},
	}
	testCases := []struct {
		name          string
		clientset     admiral.Interface
		namespace     string
		gtp           *v1.GlobalTrafficPolicy
		expectedError string
	}{
		{
			name:          "when nil clientset passed, the func should return an error",
			clientset:     nil,
			namespace:     "testNS",
			expectedError: "*admiral.Clientset is nil",
		},
		{
			name:          "when an empty namespace is passed, the func should return an error",
			clientset:     &mockAdmiralClientSet{},
			namespace:     "",
			expectedError: "namespace parameter is empty",
		},
		{
			name: "when a nil AdmiralV1Client is passed, the func should return an error",
			clientset: &mockAdmiralClientSet{
				nil,
			},
			namespace:     "testNS",
			expectedError: "*admiralv1.AdmiralV1Client is nil",
		},
		{
			name: "when an empty gtp is passed, the func should return an error",
			clientset: &mockAdmiralClientSet{
				nil,
			},
			namespace:     "testNS",
			expectedError: "*admiralv1.AdmiralV1Client is nil",
		},
		{
			name: "when valid gtp is passed, the func should run successfully without returning any errors",
			clientset: &mockAdmiralClientSet{
				&mockAdmiralV1{
					mockGlobalTrafficPolicies: &mockGlobalTrafficPolicies{
						globalTrafficPolicy: validGTP,
					},
				},
			},
			namespace:     "testNS",
			gtp:           validGTP,
			expectedError: "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			gtp, err := gtpClient.Update(context.TODO(), c.clientset, c.namespace, c.gtp, metav1.UpdateOptions{})
			if err == nil && c.expectedError != "" {
				t.Errorf("expected err: %s got nil", c.expectedError)
			}
			if err != nil && err.Error() != c.expectedError {
				t.Errorf("expected err: %s got %s", c.expectedError, err.Error())
			}
			if err == nil && gtp == nil {
				t.Error("returned gtp should not be nil")
			}
		})
	}

}

func TestDelete(t *testing.T) {
	gtpClient := &GlobalTrafficPolicy{}

	testCases := []struct {
		name          string
		clientset     admiral.Interface
		namespace     string
		gtpName       string
		expectedError string
	}{
		{
			name:          "when nil clientset passed, the func should return an error",
			clientset:     nil,
			namespace:     "testNS",
			expectedError: "*admiral.Clientset is nil",
		},
		{
			name:          "when an empty namespace is passed, the func should return an error",
			clientset:     &mockAdmiralClientSet{},
			namespace:     "",
			expectedError: "namespace parameter is empty",
		},
		{
			name: "when a nil AdmiralV1Client is passed, the func should return an error",
			clientset: &mockAdmiralClientSet{
				nil,
			},
			namespace:     "testNS",
			expectedError: "*admiralv1.AdmiralV1Client is nil",
		},
		{
			name: "when an empty gtp name is passed, the func should return an error",
			clientset: &mockAdmiralClientSet{
				nil,
			},
			namespace:     "testNS",
			gtpName:       "",
			expectedError: "*admiralv1.AdmiralV1Client is nil",
		},
		{
			name: "when valid gtp name is passed, the func should run successfully without returning any errors",
			clientset: &mockAdmiralClientSet{
				&mockAdmiralV1{
					mockGlobalTrafficPolicies: &mockGlobalTrafficPolicies{},
				},
			},
			namespace:     "testNS",
			gtpName:       "testGTP",
			expectedError: "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := gtpClient.Delete(context.TODO(), c.clientset, c.namespace, c.gtpName, metav1.DeleteOptions{})
			if err == nil && c.expectedError != "" {
				t.Errorf("expected err: %s got nil", c.expectedError)
			}
			if err != nil && err.Error() != c.expectedError {
				t.Errorf("expected err: %s got %s", c.expectedError, err.Error())
			}
		})
	}

}

type mockAdmiralClientSet struct {
	mockAdmiralV1 admiralv1.AdmiralV1Interface
}
type mockAdmiralV1 struct {
	mockGlobalTrafficPolicies admiralv1.GlobalTrafficPolicyInterface
}
type mockGlobalTrafficPolicies struct {
	globalTrafficPolicyList *v1.GlobalTrafficPolicyList
	globalTrafficPolicy     *v1.GlobalTrafficPolicy
}

func (m *mockAdmiralClientSet) AdmiralV1() admiralv1.AdmiralV1Interface {
	return m.mockAdmiralV1
}

func (*mockAdmiralClientSet) Discovery() discovery.DiscoveryInterface {
	return nil
}

func (*mockAdmiralV1) Dependencies(string) admiralv1.DependencyInterface {
	return nil
}

func (m *mockAdmiralV1) GlobalTrafficPolicies(string) admiralv1.GlobalTrafficPolicyInterface {
	return m.mockGlobalTrafficPolicies
}

func (*mockAdmiralV1) RESTClient() rest.Interface {
	return nil
}
func (m mockGlobalTrafficPolicies) Create(ctx context.Context, globalTrafficPolicy *v1.GlobalTrafficPolicy, opts metav1.CreateOptions) (*v1.GlobalTrafficPolicy, error) {
	return m.globalTrafficPolicy, nil
}

func (m mockGlobalTrafficPolicies) Update(ctx context.Context, globalTrafficPolicy *v1.GlobalTrafficPolicy, opts metav1.UpdateOptions) (*v1.GlobalTrafficPolicy, error) {
	return m.globalTrafficPolicy, nil
}

func (m mockGlobalTrafficPolicies) UpdateStatus(ctx context.Context, globalTrafficPolicy *v1.GlobalTrafficPolicy, opts metav1.UpdateOptions) (*v1.GlobalTrafficPolicy, error) {
	return nil, nil
}

func (m mockGlobalTrafficPolicies) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (m mockGlobalTrafficPolicies) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

func (m mockGlobalTrafficPolicies) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.GlobalTrafficPolicy, error) {
	return nil, nil
}

func (m mockGlobalTrafficPolicies) List(ctx context.Context, opts metav1.ListOptions) (*v1.GlobalTrafficPolicyList, error) {
	return m.globalTrafficPolicyList, nil
}

func (m mockGlobalTrafficPolicies) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (m mockGlobalTrafficPolicies) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.GlobalTrafficPolicy, err error) {
	return nil, nil
}
