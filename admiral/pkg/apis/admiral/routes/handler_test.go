package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/clusters"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/secret"
	"github.com/stretchr/testify/assert"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReturnSuccessGET(t *testing.T) {
	url := "https://admiral.com/health"
	opts := RouteOpts{}
	r := httptest.NewRequest("GET", url, strings.NewReader(""))
	w := httptest.NewRecorder()

	opts.ReturnSuccessGET(w, r)
	resp := w.Result()
	assert.Equal(t, 200, resp.StatusCode)
}

func TestReturnSuccessMetrics(t *testing.T) {
	url := "https://admiral.com/metrics"
	r := httptest.NewRequest("GET", url, strings.NewReader(""))
	w := httptest.NewRecorder()

	Noop(w, r)
	resp := w.Result()

	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetClusters(t *testing.T) {
	url := "https://admiral.com/clusters"
	rr := clusters.NewRemoteRegistry(nil, common.AdmiralParams{})
	rr.SecretController = &secret.Controller{
		Cs: &secret.ClusterStore{
			RemoteClusters: map[string]*secret.RemoteCluster{},
		},
	}
	opts := RouteOpts{
		RemoteRegistry: rr,
	}
	testCases := []struct {
		name          string
		remoteCluster map[string]*secret.RemoteCluster
		expectedErr   interface{}
		statusCode    int
	}{
		{
			name: "success with two clusters case",
			remoteCluster: map[string]*secret.RemoteCluster{
				"cluster1": {},
			},
			expectedErr: []string{"cluster1"},
			statusCode:  200,
		},
		{
			name:          "success with no cluster case",
			remoteCluster: map[string]*secret.RemoteCluster{},
			expectedErr:   "No cluster is monitored by admiral",
			statusCode:    200,
		},
	}
	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", url, strings.NewReader(""))
			w := httptest.NewRecorder()
			opts.RemoteRegistry.SecretController.Cs.RemoteClusters = c.remoteCluster
			opts.GetClusters(w, r)
			resp := w.Result()
			body, _ := ioutil.ReadAll(resp.Body)
			expectedOutput, _ := json.Marshal(c.expectedErr)
			if bytes.Compare(body, expectedOutput) != 0 {
				t.Errorf("Error mismatch. Got %v, want %v", string(body), c.expectedErr)
				t.Errorf("%d", bytes.Compare(body, expectedOutput))
			}
			if c.statusCode != 200 && resp.StatusCode != c.statusCode {
				t.Errorf("Status code mismatch. Got %v, want %v", resp.StatusCode, c.statusCode)
			}
		})
	}
}

func TestGetServiceEntriesByCluster(t *testing.T) {
	ctx := context.Background()
	url := "https://admiral.com/cluster/cluster1/serviceentries"
	opts := RouteOpts{
		RemoteRegistry: clusters.NewRemoteRegistry(nil, common.AdmiralParams{}),
	}
	fakeIstioClient := istiofake.NewSimpleClientset()
	testCases := []struct {
		name              string
		clusterName       string
		remoteControllers map[string]*clusters.RemoteController
		expectedErr       string
		statusCode        int
	}{
		{
			name:              "failure with admiral not monitored cluster",
			clusterName:       "bar",
			remoteControllers: nil,
			expectedErr:       "admiral is not monitoring cluster bar\n",
			statusCode:        404,
		},
		{
			name:              "failure with cluster not provided request",
			clusterName:       "",
			remoteControllers: nil,
			expectedErr:       "cluster name not provided as part of the request\n",
			statusCode:        400,
		},
		{
			name:        "success with no service entry for cluster",
			clusterName: "cluster1",
			remoteControllers: map[string]*clusters.RemoteController{
				"cluster1": &clusters.RemoteController{
					ServiceEntryController: &istio.ServiceEntryController{
						IstioClient: fakeIstioClient,
					},
				},
			},
			expectedErr: "no service entries configured for cluster - cluster1",
			statusCode:  200,
		},
		{
			name:        "success with service entry for cluster",
			clusterName: "cluster1",
			remoteControllers: map[string]*clusters.RemoteController{
				"cluster1": &clusters.RemoteController{
					ServiceEntryController: &istio.ServiceEntryController{
						IstioClient: fakeIstioClient,
					},
				},
			},
			expectedErr: "",
			statusCode:  200,
		},
	}
	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", url, nil)
			r = mux.SetURLVars(r, map[string]string{"clustername": c.clusterName})
			w := httptest.NewRecorder()
			rr := clusters.NewRemoteRegistry(nil, common.AdmiralParams{})
			for cId, rc := range c.remoteControllers {
				rr.PutRemoteController(cId, rc)
			}
			opts.RemoteRegistry = rr
			if c.name == "success with service entry for cluster" {
				fakeIstioClient.NetworkingV1alpha3().ServiceEntries("admiral-sync").Create(ctx, &v1alpha3.ServiceEntry{}, metaV1.CreateOptions{})
			}
			opts.GetServiceEntriesByCluster(w, r)
			resp := w.Result()
			body, _ := ioutil.ReadAll(resp.Body)
			if c.name != "success with service entry for cluster" {
				if string(body) != c.expectedErr {
					t.Errorf("Error mismatch, got: %v, want: %v", string(body), c.expectedErr)
				}
			}

			if resp.StatusCode != c.statusCode {
				t.Errorf("Status code mismatch, got: %v, want: %v", resp.StatusCode, c.statusCode)
			}
		})
	}
}

func TestGetServiceEntriesByIdentity(t *testing.T) {
	url := "https://admiral.com/identity/service1/serviceentries"
	rr := clusters.NewRemoteRegistry(nil, common.AdmiralParams{})
	rr.AdmiralCache = &clusters.AdmiralCache{
		SeClusterCache: common.NewMapOfMaps(),
	}
	opts := RouteOpts{
		RemoteRegistry: rr,
	}
	testCases := []struct {
		name        string
		identity    string
		host        string
		expectedErr string
		statusCode  int
	}{
		{
			name:        "failure with identity not provided request",
			identity:    "",
			host:        "",
			expectedErr: "Identity not provided as part of the request\n",
			statusCode:  400,
		},
		{
			name:        "success with service entry for service",
			identity:    "meshhealthcheck",
			host:        "anil-test-bdds-10-k8s-e2e.intuit.services.mesh.meshhealthcheck.mesh",
			expectedErr: "Identity not provided as part of the request\n",
			statusCode:  200,
		},
	}
	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", url, nil)
			r = mux.SetURLVars(r, map[string]string{"identity": c.identity})
			w := httptest.NewRecorder()
			if c.host != "" {
				opts.RemoteRegistry.AdmiralCache.SeClusterCache.Put(c.host, "cluster1", "cluster1")
			}
			opts.GetServiceEntriesByIdentity(w, r)
			resp := w.Result()
			body, _ := ioutil.ReadAll(resp.Body)
			if string(body) != c.expectedErr && c.name != "success with service entry for service" {
				t.Errorf("Error mismatch. Got %v, want %v", string(body), c.expectedErr)
			}
			if resp.StatusCode != c.statusCode {
				t.Errorf("Status code mismatch. Got %v, want %v", resp.StatusCode, c.statusCode)
			}
		})
	}
}

func TestGetGlobalTrafficPolicyByIdentityAndEnv(t *testing.T) {
	globalTrafficCache := &mockGlobalTrafficCache{
		identityCache: map[string]*v1.GlobalTrafficPolicy{
			"stage.testID": {
				ObjectMeta: metaV1.ObjectMeta{
					Namespace: "stage-testns",
					Name:      "stage-testapp",
					Labels:    map[string]string{"identity": "testID", "admiral.io/env": "stage"},
				},
			},
			"default.testID": {
				ObjectMeta: metaV1.ObjectMeta{
					Namespace: "default-testns",
					Name:      "default-testapp",
					Labels:    map[string]string{"identity": "testID", "admiral.io/env": "stage"},
				},
			},
		},
	}
	validOpts := RouteOpts{
		RemoteRegistry: &clusters.RemoteRegistry{
			AdmiralCache: &clusters.AdmiralCache{
				SeClusterCache:     common.NewMapOfMaps(),
				GlobalTrafficCache: globalTrafficCache,
			},
		},
	}
	testCases := []struct {
		name            string
		identity        string
		env             string
		opts            RouteOpts
		expectedStatus  int
		expectedError   string
		expectedGTPName string
	}{
		{
			name:           "nil RemoteRegistry in RouteOpts should result in InternalServerError",
			identity:       "testID",
			env:            "stage",
			opts:           RouteOpts{},
			expectedStatus: 500,
			expectedError:  "invalid remote registry cache",
		},
		{
			name:     "nil RemoteRegistry.AdmiralCache in RouteOpts should result in InternalServerError",
			identity: "testID",
			env:      "stage",
			opts: RouteOpts{
				RemoteRegistry: &clusters.RemoteRegistry{},
			},
			expectedStatus: 500,
			expectedError:  "invalid remote registry cache",
		},
		{
			name:           "missing identity path param should result in HTTP bad request",
			identity:       "",
			env:            "stage",
			opts:           validOpts,
			expectedStatus: 400,
			expectedError:  "identity not provided as part of the path param",
		},
		{
			name:            "missing env query param should return a valid 200 response with a valid GTP payload",
			identity:        "testID",
			env:             "",
			opts:            validOpts,
			expectedStatus:  200,
			expectedGTPName: "default-testapp",
		},
		{
			name:           "querying for an invalid gtp should result in a 404",
			identity:       "invalidGTP",
			env:            "stage",
			opts:           validOpts,
			expectedStatus: 404,
			expectedError:  "globaltraffic policy with identity: invalidGTP and env: stage was not found",
		},
		{
			name:            "valid GTP queried should return a valid 200 response with a valid GTP payload",
			identity:        "testID",
			env:             "stage",
			opts:            validOpts,
			expectedStatus:  200,
			expectedGTPName: "stage-testapp",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://admiral.test.com/identity/{id}/globaltrafficpolicy?env="+c.env, nil)
			r = mux.SetURLVars(r, map[string]string{"identity": c.identity})
			w := httptest.NewRecorder()
			c.opts.GetGlobalTrafficPolicyByIdentityAndEnv(w, r)
			res := w.Result()
			data, err := ioutil.ReadAll(res.Body)
			if err != nil {
				t.Error(err)
			}
			if res.StatusCode != c.expectedStatus {
				t.Errorf("expected http status %d got %d", c.expectedStatus, res.StatusCode)
			}
			if c.expectedError != "" {
				responseJSON := make(map[string]string)
				json.Unmarshal(data, &responseJSON)
				if responseJSON["error"] != c.expectedError {
					t.Errorf("expected error '%s' got '%s'", c.expectedError, responseJSON["error"])
				}
			} else {
				var responseGTP *v1.GlobalTrafficPolicy
				json.Unmarshal(data, &responseGTP)
				if responseGTP == nil {
					t.Error("expected response GTP to be not nil")
				}
				if c.expectedGTPName != responseGTP.Name {
					t.Errorf("expected GTP %s got GTP %s", c.expectedGTPName, responseGTP.Name)
				}
			}
			res.Body.Close()
		})
	}

}

type mockGlobalTrafficCache struct {
	identityCache map[string]*v1.GlobalTrafficPolicy
}

func (m *mockGlobalTrafficCache) GetFromIdentity(identity string, environment string) (*v1.GlobalTrafficPolicy, error) {
	return m.identityCache[common.ConstructKeyWithEnvAndIdentity(environment, identity)], nil
}

func (*mockGlobalTrafficCache) Put(*v1.GlobalTrafficPolicy) error {
	return nil
}

func (*mockGlobalTrafficCache) Delete(string, string) error {
	return nil
}
