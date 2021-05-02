package routes

import (
	"bytes"
	"encoding/json"
	"github.com/istio-ecosystem/admiral/admiral/pkg/clusters"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/secret"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
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
		method string
		remoteCluster map[string]*secret.RemoteCluster
		expectedErr interface{}
		statusCode int
	}{
		{
			name:   "success with 2 clusters case",
			method: "GET",
			remoteCluster:     map[string]*secret.RemoteCluster{
				"cluster1": &secret.RemoteCluster{},
				"cluster2": &secret.RemoteCluster{},
			},
			expectedErr:      []string{"cluster1", "cluster2"},
			statusCode:        200,
		},
		{
			name:   "success with no cluster case",
			method: "GET",
			remoteCluster:     map[string]*secret.RemoteCluster{},
			expectedErr:       "No cluster is monitored by admiral",
			statusCode:        200,
		},
	}
	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			r:= httptest.NewRequest(c.method, url, strings.NewReader(""))
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