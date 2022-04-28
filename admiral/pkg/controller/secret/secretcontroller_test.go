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
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"k8s.io/client-go/rest"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	pkgtest "github.com/istio-ecosystem/admiral/admiral/pkg/test"
)

const secretName string = "testSecretName"
const secretNameSpace string = "istio-system"

var testCreateControllerCalled bool
var testDeleteControllerCalled bool

func makeSecret(secret, clusterID string, kubeconfig []byte) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret,
			Namespace: secretNameSpace,
			Labels: map[string]string{
				filterLabel: "true",
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

func addCallback(config *rest.Config, id string, resyncPeriod time.Duration) error {
	mu.Lock()
	defer mu.Unlock()
	added = id
	return nil
}

func updateCallback(config *rest.Config, id string, resyncPeriod time.Duration) error {
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
	secret := v1.Secret{
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
	_, err := k8s.CoreV1().Secrets(secretNameSpace).Create(&secret)
	return err
}

func deleteMultiClusterSecret(k8s *fake.Clientset) error {
	var immediate int64

	return k8s.CoreV1().Secrets(secretNameSpace).Delete(
		secretName, &metav1.DeleteOptions{GracePeriodSeconds: &immediate})
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

func verifyControllerDeleted(t *testing.T, timeoutName string) {
	pkgtest.NewEventualOpts(10*time.Millisecond, 5*time.Second).Eventually(t, timeoutName, func() bool {
		return testDeleteControllerCalled == true
	})
}

func verifyControllerCreated(t *testing.T, timeoutName string) {
	pkgtest.NewEventualOpts(10*time.Millisecond, 5*time.Second).Eventually(t, timeoutName, func() bool {
		return testCreateControllerCalled == true
	})
}

/*
func Test_SecretController(t *testing.T) {
	LoadKubeConfig = mockLoadKubeConfig

	clientset := fake.NewSimpleClientset()

	// Start the secret controller and sleep to allow secret process to start.
	err := StartSecretController(
		clientset, testCreateController, testDeleteController, secretNameSpace, context.TODO(), "")
	if err != nil {
		t.Fatalf("Could not start secret controller: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Create the multicluster secret.
	err = createMultiClusterSecret(clientset)
	if err != nil {
		t.Fatalf("Unexpected error on secret create: %v", err)
	}

	verifyControllerCreated(t, "Create remote secret controller")

	if testDeleteControllerCalled != false {
		t.Fatalf("Test failed on create secret, delete callback function called")
	}

	// Reset test variables and delete the multicluster secret.
	testCreateControllerCalled = false
	testDeleteControllerCalled = false

	err = deleteMultiClusterSecret(clientset)
	if err != nil {
		t.Fatalf("Unexpected error on secret delete: %v", err)
	}

	// Test - Verify that the remote controller has been removed.
	verifyControllerDeleted(t, "delete remote secret controller")

	// Test
	if testCreateControllerCalled != false {
		t.Fatalf("Test failed on delete secret, create callback function called")
	}
}*/

func Test_SecretController(t *testing.T) {
	g := NewWithT(t)

	LoadKubeConfig = mockLoadKubeConfig

	clientset := fake.NewSimpleClientset()

	var (
		secret0                        = makeSecret("s0", "c0", []byte("kubeconfig0-0"))
		secret0UpdateKubeconfigChanged = makeSecret("s0", "c0", []byte("kubeconfig0-1"))
		secret1                        = makeSecret("s1", "c1", []byte("kubeconfig1-0"))
	)

	p := common.AdmiralParams{MetricsEnabled: true}
	common.InitializeConfig(p)

	steps := []struct {
		// only set one of these per step. The others should be nil.
		add    *v1.Secret
		update *v1.Secret
		delete *v1.Secret

		// only set one of these per step. The others should be empty.
		wantAdded   string
		wantUpdated string
		wantDeleted string

		// clusters-monitored metric
		clustersMonitored float64
	}{
		{add: secret0, wantAdded: "c0", clustersMonitored: 1},
		{update: secret0UpdateKubeconfigChanged, wantUpdated: "c0", clustersMonitored: 1},
		{add: secret1, wantAdded: "c1", clustersMonitored: 2},
		{delete: secret0, wantDeleted: "c0", clustersMonitored: 1},
		{delete: secret1, wantDeleted: "c1", clustersMonitored: 0},
	}

	// Start the secret controller and sleep to allow secret process to start.
	// The assertion ShouldNot(BeNil()) make sure that start secret controller return a not nil controller and nil error
	registry := prometheus.DefaultGatherer
	g.Expect(
		StartSecretController(clientset, addCallback, updateCallback, deleteCallback, secretNameSpace, context.TODO(), "")).
		ShouldNot(BeNil())

	for i, step := range steps {
		resetCallbackData()

		t.Run(fmt.Sprintf("[%v]", i), func(t *testing.T) {
			g := NewWithT(t)

			switch {
			case step.add != nil:
				_, err := clientset.CoreV1().Secrets(secretNameSpace).Create(step.add)
				g.Expect(err).Should(BeNil())
			case step.update != nil:
				_, err := clientset.CoreV1().Secrets(secretNameSpace).Update(step.update)
				g.Expect(err).Should(BeNil())
			case step.delete != nil:
				g.Expect(clientset.CoreV1().Secrets(secretNameSpace).Delete(step.delete.Name, &metav1.DeleteOptions{})).
					Should(Succeed())
			}

			switch {
			case step.wantAdded != "":
				g.Eventually(func() string {
					mu.Lock()
					defer mu.Unlock()
					return added
				}, 10*time.Second).Should(Equal(step.wantAdded))
			case step.wantUpdated != "":
				g.Eventually(func() string {
					mu.Lock()
					defer mu.Unlock()
					return updated
				}, 10*time.Second).Should(Equal(step.wantUpdated))
			case step.wantDeleted != "":
				g.Eventually(func() string {
					mu.Lock()
					defer mu.Unlock()
					return deleted
				}, 10*time.Second).Should(Equal(step.wantDeleted))
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
