package routes

import (
	"bytes"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/istio-ecosystem/admiral/admiral/pkg/clusters"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/secret"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReturnSuccessGET (t *testing.T) {
	url := "https://admiral.com/health"
	opts := RouteOpts{}
	r := httptest.NewRequest("GET", url, strings.NewReader(""))
	w := httptest.NewRecorder()

	opts.ReturnSuccessGET(w, r)
	resp := w.Result()
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetClusters (t *testing.T) {
	url := "https://admiral.com/clusters"
	opts := RouteOpts{
		RemoteRegistry: &clusters.RemoteRegistry{
			SecretController: &secret.Controller{
				Cs: &secret.ClusterStore{
					RemoteClusters: map[string]*secret.RemoteCluster{},
				},
			},
		},
	}
	testCases := []struct {
		name string
		remoteCluster map[string]*secret.RemoteCluster
		expectedErr interface{}
		statusCode int
	}{
		{
			name:   "success with 2 clusters case",
			remoteCluster:     map[string]*secret.RemoteCluster{
				"cluster1": &secret.RemoteCluster{},
				"cluster2": &secret.RemoteCluster{},
			},
			expectedErr:      []string{"cluster1", "cluster2"},
			statusCode:        200,
		},
		{
			name:   "success with no cluster case",
			remoteCluster:     map[string]*secret.RemoteCluster{},
			expectedErr:       "No cluster is monitored by admiral",
			statusCode:        200,
		},
	}
	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			r:= httptest.NewRequest("GET", url, strings.NewReader(""))
			w := httptest.NewRecorder()
			opts.RemoteRegistry.SecretController.Cs.RemoteClusters = c.remoteCluster
			opts.GetClusters(w, r)
			resp := w.Result()
			body, _ := ioutil.ReadAll(resp.Body)
			expectedOutput, _ := json.Marshal(c.expectedErr)
			if bytes.Compare(body, expectedOutput) != 0 {
				t.Errorf("Error mismatch. Got %v, want %v", string(body), c.expectedErr)
				t.Errorf("%d",bytes.Compare(body, expectedOutput))
			}
			if c.statusCode != 200 && resp.StatusCode != c.statusCode {
				t.Errorf("Status code mismatch. Got %v, want %v", resp.StatusCode, c.statusCode)
			}
		})
	}
}

func TestGetServiceEntriesByCluster (t *testing.T) {
	url := "https://admiral.com/cluster/cluster1/serviceentries"
	opts := RouteOpts{
		RemoteRegistry: &clusters.RemoteRegistry{},
	}
	fakeIstioClient := istiofake.NewSimpleClientset()
	testCases := []struct {
		name string
		clusterName string
		remoteControllers map[string]*clusters.RemoteController
		expectedErr string
		statusCode int
	}{
		{
			name:             "failure with admiral not monitored cluster",
		    clusterName:       "bar",
			remoteControllers:     nil,
			expectedErr:      "Admiral is not monitoring cluster bar\n",
			statusCode:        404,
		},
		{
			name:             "failure with admiral not monitored cluster",
			clusterName:       "",
			remoteControllers:     nil,
			expectedErr:      "Cluster name not provided as part of the request\n",
			statusCode:        400,
		},
		{
			name:             "success with no service entry for cluster",
			clusterName:      "cluster1",
			remoteControllers:     map[string]*clusters.RemoteController{
				"cluster1": &clusters.RemoteController{
					ServiceEntryController:    &istio.ServiceEntryController{
						IstioClient: fakeIstioClient,
					},
				},
			},
			expectedErr:      "No service entries configured for cluster - cluster1",
			statusCode:        200,
		},
		{
			name:             "success with service entry for cluster",
			clusterName:      "cluster1",
			remoteControllers:     map[string]*clusters.RemoteController{
				"cluster1": &clusters.RemoteController{
					ServiceEntryController:    &istio.ServiceEntryController{
						IstioClient: fakeIstioClient,
					},
				},
			},
			expectedErr:      "",
			statusCode:        200,
		},
	}
	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			r:= httptest.NewRequest("GET", url, nil)
			r = mux.SetURLVars(r, map[string]string{"clustername": c.clusterName})
			w := httptest.NewRecorder()
			opts.RemoteRegistry.RemoteControllers = c.remoteControllers
			if c.name == "success with service entry for cluster" {
				fakeIstioClient.NetworkingV1alpha3().ServiceEntries("admiral-sync").Create(&v1alpha3.ServiceEntry{})
			}
			opts.GetServiceEntriesByCluster(w, r)
			resp := w.Result()
			body, _ := ioutil.ReadAll(resp.Body)
			if string(body) != c.expectedErr && c.name != "success with service entry for cluster" {
				t.Errorf("Error mismatch. Got %v, want %v", string(body), c.expectedErr)
			}
			if resp.StatusCode != c.statusCode {
				t.Errorf("Status code mismatch. Got %v, want %v", resp.StatusCode, c.statusCode)
			}
		})
	}
}