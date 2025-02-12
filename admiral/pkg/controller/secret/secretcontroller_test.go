// Copyright 2018 Istio Authors.
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

package secret

import (
	"context"
	"fmt"
	"k8s.io/client-go/kubernetes"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/util"
	coreV1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/stretchr/testify/assert"
)

const (
	secretName      string = "testSecretName"
	secretNameSpace string = "istio-system"
)

var (
	testCreateControllerCalled bool
	testDeleteControllerCalled bool
)

func makeSecret(secret, clusterID string, kubeconfig []byte) *coreV1.Secret {
	return &coreV1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret,
			Namespace: secretNameSpace,
			Labels: map[string]string{
				common.GetSecretFilterTags(): "true",
			},
		},
		Data: map[string][]byte{
			clusterID: kubeconfig,
		},
	}
}

func makeSecretWithCustomFilterTag(secret, clusterID string, kubeconfig []byte, secretFilterTag string) *coreV1.Secret {
	return &coreV1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret,
			Namespace: secretNameSpace,
			Labels: map[string]string{
				secretFilterTag: "true",
			},
		},
		Data: map[string][]byte{
			clusterID: kubeconfig,
		},
	}
}

var (
	mu      sync.Mutex
	added   string
	updated string
	deleted string
)

func addCallback(config *rest.Config, id string, resyncPeriod util.ResyncIntervals) error {
	mu.Lock()
	defer mu.Unlock()
	added = id
	return nil
}

func updateCallback(config *rest.Config, id string, resyncPeriod util.ResyncIntervals) error {
	mu.Lock()
	defer mu.Unlock()
	updated = id
	return nil
}

func deleteCallback(id string) error {
	mu.Lock()
	defer mu.Unlock()
	deleted = id
	return nil
}

func resetCallbackData() {
	added = ""
	updated = ""
	deleted = ""
}

func testCreateController(clientConfig *rest.Config, clusterID string, resyncPeriod time.Duration) error {
	testCreateControllerCalled = true
	return nil
}

func testDeleteController(clusterID string) error {
	testDeleteControllerCalled = true
	return nil
}

func createMultiClusterSecret(k8s *fake.Clientset) error {
	data := map[string][]byte{}
	secret := coreV1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: secretNameSpace,
			Labels: map[string]string{
				"istio/multiCluster": "true",
			},
		},
		Data: map[string][]byte{},
	}

	data["testRemoteCluster"] = []byte("Test")
	secret.Data = data
	ctx := context.Background()
	_, err := k8s.CoreV1().Secrets(secretNameSpace).Create(ctx, &secret, metav1.CreateOptions{})
	return err
}

func deleteMultiClusterSecret(k8s *fake.Clientset) error {
	var immediate int64

	ctx := context.Background()
	return k8s.CoreV1().Secrets(secretNameSpace).Delete(ctx,
		secretName, metav1.DeleteOptions{GracePeriodSeconds: &immediate})
}

func mockLoadKubeConfig(kubeconfig []byte) (*clientcmdapi.Config, error) {
	config := clientcmdapi.NewConfig()
	config.Clusters["clean"] = &clientcmdapi.Cluster{
		Server: "https://anything.com:8080",
	}
	config.Contexts["clean"] = &clientcmdapi.Context{
		Cluster: "clean",
	}
	config.CurrentContext = "clean"
	return config, nil
}

func Test_SecretFilterTags(t *testing.T) {
	g := NewWithT(t)

	LoadKubeConfig = mockLoadKubeConfig

	secretFilterTag := "admiral/test-filter-tag"

	p := common.AdmiralParams{
		MetricsEnabled:   true,
		SecretFilterTags: secretFilterTag,
	}

	common.InitializeConfig(p)

	secret := makeSecretWithCustomFilterTag("s0", "c0", []byte("kubeconfig0-0"), secretFilterTag)

	g.Expect(common.GetSecretFilterTags()).Should(Equal(secretFilterTag))       // Check if the secret filter tag is set correctly on the config
	g.Expect(secret.Labels[common.GetSecretFilterTags()]).Should(Equal("true")) // Check if the secret filter tag matches the one set on the config to watch.

}

func Test_SecretFilterTagsMismatch(t *testing.T) {
	g := NewWithT(t)

	LoadKubeConfig = mockLoadKubeConfig

	secretFilterTag := "admiral/test-filter-tag"

	p := common.AdmiralParams{
		MetricsEnabled:   true,
		SecretFilterTags: secretFilterTag,
	}

	common.InitializeConfig(p)

	secret := makeSecretWithCustomFilterTag("s0", "c0", []byte("kubeconfig0-0"), "admiral/other-filter-tag")

	g.Expect(common.GetSecretFilterTags()).Should(Equal(secretFilterTag))   // Check if the secret filter tag is set correctly on the config
	g.Expect(secret.Labels[common.GetSecretFilterTags()]).Should(Equal("")) // Check if the secret filter tag doesnt match the one set on the config to watch, hence it should be empty.

}

/*
	func Test_SecretController(t *testing.T) {
		g := NewWithT(t)

		LoadKubeConfig = mockLoadKubeConfig

		clientset := fake.NewSimpleClientset()

		p := common.AdmiralParams{
			MetricsEnabled:   true,
			SecretFilterTags: "admiral/sync",
		}
		common.InitializeConfig(p)

		var (
			secret0 = makeSecret("s0", "c0", []byte("kubeconfig0-0"))
			//secret0UpdateKubeconfigChanged = makeSecret("s0", "c0", []byte("kubeconfig0-1"))
			secret1 = makeSecret("s1", "c1", []byte("kubeconfig1-0"))
		)

		steps := []struct {
			// only set one of these per step. The others should be nil.
			add    *coreV1.Secret
			update *coreV1.Secret
			delete *coreV1.Secret

			// only set one of these per step. The others should be empty.
			wantAdded   string
			wantUpdated string
			wantDeleted string

			// clusters-monitored metric
			clustersMonitored float64
		}{
			{add: secret0, wantAdded: "c0", clustersMonitored: 1},
			//{update: secret0UpdateKubeconfigChanged, wantUpdated: "c0", clustersMonitored: 1},
			{add: secret1, wantAdded: "c1", clustersMonitored: 2},
			{delete: secret0, wantDeleted: "c0", clustersMonitored: 1},
			{delete: secret1, wantDeleted: "c1", clustersMonitored: 0},
		}

		// Start the secret controller and sleep to allow secret process to start.
		// The assertion ShouldNot(BeNil()) make sure that start secret controller return a not nil controller and nil error
		registry := prometheus.DefaultGatherer
		g.Expect(
			StartSecretController(context.TODO(), clientset, addCallback, updateCallback, deleteCallback, secretNameSpace, common.AdmiralProfileDefault, "")).
			ShouldNot(BeNil())

		ctx := context.Background()
		for i, step := range steps {
			resetCallbackData()

			t.Run(fmt.Sprintf("[%v]", i), func(t *testing.T) {
				g := NewWithT(t)

				switch {
				case step.add != nil:
					_, err := clientset.CoreV1().Secrets(secretNameSpace).Create(ctx, step.add, metav1.CreateOptions{})
					g.Expect(err).Should(BeNil())
				case step.update != nil:
					_, err := clientset.CoreV1().Secrets(secretNameSpace).Update(ctx, step.update, metav1.UpdateOptions{})
					g.Expect(err).Should(BeNil())
				case step.delete != nil:
					g.Expect(clientset.CoreV1().Secrets(secretNameSpace).Delete(ctx, step.delete.Name, metav1.DeleteOptions{})).
						Should(Succeed())
				}

				switch {
				case step.wantAdded != "":
					g.Eventually(func() string {
						mu.Lock()
						defer mu.Unlock()
						return added
					}, 60*time.Second).Should(Equal(step.wantAdded))
				case step.wantUpdated != "":
					g.Eventually(func() string {
						mu.Lock()
						defer mu.Unlock()
						return updated
					}, 60*time.Second).Should(Equal(step.wantUpdated))
				case step.wantDeleted != "":
					g.Eventually(func() string {
						mu.Lock()
						defer mu.Unlock()
						return deleted
					}, 60*time.Second).Should(Equal(step.wantDeleted))
				default:
					g.Consistently(func() bool {
						mu.Lock()
						defer mu.Unlock()
						return added == "" && updated == "" && deleted == ""
					}).Should(Equal(true))
				}

				g.Eventually(func() float64 {
					mf, _ := registry.Gather()
					var clustersMonitored *io_prometheus_client.MetricFamily
					for _, m := range mf {
						if *m.Name == "clusters_monitored" {
							clustersMonitored = m
						}
					}
					return *clustersMonitored.Metric[0].Gauge.Value
				}).Should(Equal(step.clustersMonitored))
			})
		}
	}
*/
func TestGetShardNameFromClusterSecret(t *testing.T) {
	cases := []struct {
		name            string
		secret          *corev1.Secret
		stateSyncerMode bool
		want            string
		wantErr         error
	}{
		{
			name: "Given secret is empty" +
				"When function is invoked, " +
				"It should return an error",
			stateSyncerMode: true,
			secret:          nil,
			want:            "",
			wantErr:         fmt.Errorf("nil secret passed"),
		},
		{
			name: "Given secret is empty, " +
				"And, state syncer mode is false, " +
				"When function is invoked, " +
				"It should return an error",
			secret:  nil,
			want:    "",
			wantErr: nil,
		},
		{
			name: "Given secret is valid, but does not have annotations" +
				"When function is invoked, " +
				"It should return an error",
			stateSyncerMode: true,
			secret: &coreV1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: secretNameSpace,
					Labels: map[string]string{
						common.GetSecretFilterTags(): "true",
					},
				},
			},
			want:    "",
			wantErr: fmt.Errorf("no annotations found on secret=%s", secretName),
		},
		{
			name: "Given secret is valid, and has valid annotations" +
				"When function is invoked, " +
				"It should return a valid value, without any error",
			stateSyncerMode: true,
			secret: &coreV1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: secretNameSpace,
					Annotations: map[string]string{
						util.SecretShardKey: "shard1",
					},
				},
			},
			want:    "shard1",
			wantErr: nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			common.ResetSync()
			common.InitializeConfig(common.AdmiralParams{
				AdmiralStateSyncerMode: c.stateSyncerMode,
			})
			got, err := getShardNameFromClusterSecret(c.secret)
			if got != c.want {
				t.Errorf("want=%s, got=%s", c.want, got)
			}
			if !reflect.DeepEqual(err, c.wantErr) {
				t.Errorf("want=%v, got=%v", c.wantErr, err)
			}
		})
	}
}

func TestNewClustersStore(t *testing.T) {
	t.Parallel()
	store := newClustersStore()
	assert.NotNil(t, store)
}

// Initializes a new Controller with default secret resolver when admiralProfile is "default"
func TestNewControllerWithDefaultProfile(t *testing.T) {
	kubeclientset := &kubernetes.Clientset{}
	namespace := "test-namespace"
	cs := &ClusterStore{}
	addCallback := func(config *rest.Config, dataKey string, resyncPeriod util.ResyncIntervals) error { return nil }
	updateCallback := func(config *rest.Config, dataKey string, resyncPeriod util.ResyncIntervals) error { return nil }
	removeCallback := func(dataKey string) error { return nil }
	admiralProfile := common.AdmiralProfileDefault
	secretResolverConfig := ""

	controller := NewController(kubeclientset, namespace, cs, addCallback, updateCallback, removeCallback, admiralProfile, secretResolverConfig)

	if controller == nil {
		t.Fatalf("Expected controller to be initialized, got nil")
	}

	if controller.secretResolver == nil {
		t.Fatalf("Expected secret resolver to be initialized, got nil")
	}
}

// Handles unrecognized admiralProfile by logging an error and returning nil
func TestNewControllerWithUnrecognizedProfile(t *testing.T) {
	kubeclientset := &kubernetes.Clientset{}
	namespace := "test-namespace"
	cs := &ClusterStore{}
	addCallback := func(config *rest.Config, dataKey string, resyncPeriod util.ResyncIntervals) error { return nil }
	updateCallback := func(config *rest.Config, dataKey string, resyncPeriod util.ResyncIntervals) error { return nil }
	removeCallback := func(dataKey string) error { return nil }
	admiralProfile := "unknown-profile"
	secretResolverConfig := ""

	controller := NewController(kubeclientset, namespace, cs, addCallback, updateCallback, removeCallback, admiralProfile, secretResolverConfig)

	if controller != nil {
		t.Fatalf("Expected controller to be nil for unrecognized profile, got non-nil")
	}
}

// Successfully initializes a new Controller with valid parameters
func TestStartSecretControllerInitialization(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockK8s := &kubernetes.Clientset{}
	addCallback := func(config *rest.Config, dataKey string, resyncPeriod util.ResyncIntervals) error { return nil }
	updateCallback := func(config *rest.Config, dataKey string, resyncPeriod util.ResyncIntervals) error { return nil }
	removeCallback := func(dataKey string) error { return nil }
	namespace := "default"
	admiralProfile := common.AdmiralProfileDefault
	secretResolverConfig := ""

	controller, err := StartSecretController(ctx, mockK8s, addCallback, updateCallback, removeCallback, namespace, admiralProfile, secretResolverConfig)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if controller == nil {
		t.Fatalf("Expected controller to be initialized, got nil")
	}
}

// Returns error when kubeConfig is empty
func TestCreateRemoteClusterWithEmptyKubeConfig(t *testing.T) {
	// Arrange
	controller := &Controller{}

	// Act
	remoteCluster, restConfig, err := controller.createRemoteCluster([]byte(""), "secretName", "clusterID", "namespace")

	// Assert
	assert.Nil(t, remoteCluster)
	assert.Nil(t, restConfig)
	assert.Error(t, err)
	assert.Equal(t, "kubeconfig is empty", err.Error())
}
