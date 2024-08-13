package clusters

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/istio-ecosystem/admiral/admiral/pkg/registry"
)

func TestUpdateClusterIdentityState(t *testing.T) {
	var (
		sourceCluster1          = "cluster1"
		foobarIdentity          = "org.foobar.service"
		helloWorldIdentity      = "org.helloworld.service"
		remoteRegistryHappyCase = &RemoteRegistry{
			ClusterIdentityStoreHandler: registry.NewClusterIdentityStoreHandler(),
			AdmiralCache: &AdmiralCache{
				SourceToDestinations: &sourceToDestinations{
					sourceDestinations: map[string][]string{
						foobarIdentity: {helloWorldIdentity},
					},
					mutex: &sync.Mutex{},
				},
			},
		}
	)
	cases := []struct {
		name           string
		remoteRegistry *RemoteRegistry
		sourceClusters []string
		assertFunc     func() error
		expectedErr    error
	}{
		{
			name: "Given remote registry is empty, " +
				"When the function is called, " +
				"It should return an error",
			expectedErr: fmt.Errorf("remote registry is not initialized"),
		},
		{
			name: "Given remote registry admiral cache is empty, " +
				"When the function is called, " +
				"It should return an error",
			remoteRegistry: &RemoteRegistry{},
			expectedErr:    fmt.Errorf("admiral cache is not initialized"),
		},
		{
			name: "Given source to destination cache is empty, " +
				"When the function is called, " +
				"It should return an error",
			remoteRegistry: &RemoteRegistry{
				AdmiralCache: &AdmiralCache{},
			},
			expectedErr: fmt.Errorf("source to destination cache is not populated"),
		},
		{
			name: "Given all caches are initialized, " +
				"When the function is called for an asset '" + foobarIdentity + "', which is present in cluster A, " +
				"And which has 1 destination asset '" + helloWorldIdentity + "', " +
				"It should update the cluster identity, such that, " +
				"cluster A has two assets - '" + foobarIdentity + "' as a source asset, " +
				"and '" + helloWorldIdentity + "' as a regular asset",
			sourceClusters: []string{sourceCluster1},
			remoteRegistry: remoteRegistryHappyCase,
			assertFunc: func() error {
				identityStore, err := remoteRegistryHappyCase.ClusterIdentityStoreHandler.GetAllIdentitiesForCluster(sourceCluster1)
				if err != nil {
					return err
				}
				if len(identityStore.Store) != 2 {
					return fmt.Errorf("expected two identities, got=%v", len(identityStore.Store))
				}
				var (
					foundFoobar     bool
					foundHelloWorld bool
				)
				for identity, clusterIdentity := range identityStore.Store {
					if identity == foobarIdentity {
						if !clusterIdentity.SourceIdentity {
							return fmt.Errorf("expected '%s' to be a source identity, but it was not", foobarIdentity)
						}
						foundFoobar = true
					}
					if identity == helloWorldIdentity {
						if clusterIdentity.SourceIdentity {
							return fmt.Errorf("expected '%s' to be a regular identity, but it was a source identity", helloWorldIdentity)
						}
						foundHelloWorld = true
					}
				}
				if !foundFoobar {
					return fmt.Errorf("expected to find 'foobar', but it was not found")
				}
				if !foundHelloWorld {
					return fmt.Errorf("expected to find 'helloWorld', but it was not found")
				}
				return nil
			},
			expectedErr: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := updateClusterIdentityCache(
				c.remoteRegistry, c.sourceClusters, foobarIdentity,
			)
			if !reflect.DeepEqual(err, c.expectedErr) {
				t.Errorf("got=%v, want=%v", err, c.expectedErr)
			}
			if c.expectedErr == nil && c.assertFunc != nil {
				// validate the configuration got updated
				err = c.assertFunc()
				if err != nil {
					t.Errorf("got=%v, want=nil", err)
				}
			}
		})
	}
}
