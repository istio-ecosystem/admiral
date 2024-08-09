package clusters

import (
	"context"
	"fmt"
	"testing"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/stretchr/testify/assert"
	networkingAlpha3 "istio.io/api/networking/v1alpha3"
	apiMachineryMetaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func setupForGlobalTrafficHandlerTests() {
	typeTestSingleton.Do(func() {
		common.ResetSync()
		common.InitializeConfig(admiralParamsForTypesTests())
	})
}

func TestHandleEventForGlobalTrafficPolicy(t *testing.T) {
	setupForGlobalTrafficHandlerTests()
	ctx := context.Background()
	event := admiral.EventType("Add")
	p := common.AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
	}
	registry, _ := InitAdmiral(context.Background(), p)

	seFunc := func(ctx context.Context, event admiral.EventType, env string, sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*networkingAlpha3.ServiceEntry, error) {
		return nil, nil
	}

	seErrFunc := func(ctx context.Context, event admiral.EventType, env string, sourceIdentity string, remoteRegistry *RemoteRegistry) (map[string]*networkingAlpha3.ServiceEntry, error) {
		return nil, fmt.Errorf("Error")
	}
	cases := []struct {
		name      string
		gtp       *v1.GlobalTrafficPolicy
		seFunc    ModifySEFunc
		doesError bool
	}{
		{
			name: "missing identity label in GTP should result in error being returned by the handler",
			gtp: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			seFunc:    seFunc,
			doesError: true,
		},
		{
			name: "empty identity label in GTP should result in error being returned by the handler",
			gtp: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Labels:      map[string]string{"identity": ""},
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			seFunc:    seFunc,
			doesError: true,
		},
		{
			name: "valid GTP config which is expected to pass",
			gtp: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Labels:      map[string]string{"identity": "testapp"},
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			seFunc:    seFunc,
			doesError: false,
		},
		{
			name: "Given a valid GTP config, " +
				"And modifyServiceEntryForNewServiceOrPod returns an error" +
				"Then, the function would return an error",
			gtp: &v1.GlobalTrafficPolicy{
				ObjectMeta: apiMachineryMetaV1.ObjectMeta{
					Name:        "testgtp",
					Labels:      map[string]string{"identity": "testapp"},
					Annotations: map[string]string{"admiral.io/env": "testenv"},
				},
			},
			seFunc:    seErrFunc,
			doesError: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := HandleEventForGlobalTrafficPolicy(ctx, event, c.gtp, registry, "testcluster", c.seFunc)
			assert.Equal(t, err != nil, c.doesError)
		})
	}
}
