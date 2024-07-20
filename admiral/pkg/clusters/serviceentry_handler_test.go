package clusters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"
	log "github.com/sirupsen/logrus"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/stretchr/testify/assert"
	"istio.io/api/networking/v1alpha3"
	v1alpha32 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istioFake "istio.io/client-go/pkg/clientset/versioned/fake"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSkipDestructiveUpdate(t *testing.T) {
	admiralParams := common.AdmiralParams{
		CacheReconcileDuration: 5 * time.Minute,
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	twoEndpointSe := v1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{"240.10.1.1"},
		Ports: []*v1alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      v1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global-west", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
			{Address: "dummy.admiral.global-east", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
		},
	}
	twoEndpointSeUpdated := v1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{"240.10.1.1"},
		Ports: []*v1alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      v1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global-west", Ports: map[string]uint32{"http": 90}, Locality: "us-west-2"},
			{Address: "dummy.admiral.global-east", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
		},
	}
	oneEndpointSe := v1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{"240.10.1.1"},
		Ports: []*v1alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      v1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global-west", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
		},
	}
	newSeTwoEndpoints := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se1", Namespace: "random"},
		//nolint
		Spec: twoEndpointSe,
	}
	newSeTwoEndpointsUpdated := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se1", Namespace: "random"},
		//nolint
		Spec: twoEndpointSeUpdated,
	}
	newSeOneEndpoint := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se1", Namespace: "random"},
		//nolint
		Spec: oneEndpointSe,
	}
	oldSeTwoEndpoints := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se1", Namespace: "random"},
		//nolint
		Spec: twoEndpointSe,
	}
	oldSeOneEndpoint := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se1", Namespace: "random"},
		//nolint
		Spec: oneEndpointSe,
	}
	rcWarmupPhase := &RemoteController{
		StartTime: time.Now(),
	}
	rcNotinWarmupPhase := &RemoteController{
		StartTime: time.Now().Add(time.Duration(-21) * time.Minute),
	}
	//Struct of test case info. Name is required.
	testCases := []struct {
		name            string
		rc              *RemoteController
		newSe           *v1alpha32.ServiceEntry
		oldSe           *v1alpha32.ServiceEntry
		skipDestructive bool
		diff            string
	}{
		{
			name:            "Should return false when in warm up phase but not destructive",
			rc:              rcWarmupPhase,
			newSe:           newSeOneEndpoint,
			oldSe:           oldSeOneEndpoint,
			skipDestructive: false,
			diff:            "",
		},
		{
			name:            "Should return true when in warm up phase but is destructive",
			rc:              rcWarmupPhase,
			newSe:           newSeOneEndpoint,
			oldSe:           oldSeTwoEndpoints,
			skipDestructive: true,
			diff:            "Delete",
		},
		{
			name:            "Should return false when not in warm up phase but is destructive",
			rc:              rcNotinWarmupPhase,
			newSe:           newSeOneEndpoint,
			oldSe:           oldSeTwoEndpoints,
			skipDestructive: false,
			diff:            "Delete",
		},
		{
			name:            "Should return false when in warm up phase but is constructive",
			rc:              rcWarmupPhase,
			newSe:           newSeTwoEndpoints,
			oldSe:           oldSeOneEndpoint,
			skipDestructive: false,
			diff:            "Add",
		},
		{
			name:            "Should return false when not in warm up phase but endpoints updated",
			rc:              rcNotinWarmupPhase,
			newSe:           newSeTwoEndpointsUpdated,
			oldSe:           oldSeTwoEndpoints,
			skipDestructive: false,
			diff:            "Update",
		},
		{
			name:            "Should return true when in warm up phase but endpoints are updated (destructive)",
			rc:              rcWarmupPhase,
			newSe:           newSeTwoEndpointsUpdated,
			oldSe:           oldSeTwoEndpoints,
			skipDestructive: true,
			diff:            "Update",
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			skipDestructive, diff := skipDestructiveUpdate(c.rc, c.newSe, c.oldSe)
			if skipDestructive == c.skipDestructive {
				//perfect
			} else {
				t.Errorf("Result Failed. Got %v, expected %v", skipDestructive, c.skipDestructive)
			}
			if c.diff == "" || (c.diff != "" && strings.Contains(diff, c.diff)) {
				//perfect
			} else {
				t.Errorf("Diff Failed. Got %v, expected %v", diff, c.diff)
			}
		})
	}
}

func TestAddUpdateServiceEntry(t *testing.T) {
	var (
		ctx       = context.Background()
		ctxLogger = log.WithFields(log.Fields{
			"type": "modifySE",
		})
		fakeIstioClient = istioFake.NewSimpleClientset()
		seCtrl          = &istio.ServiceEntryController{
			IstioClient: fakeIstioClient,
		}
	)

	twoEndpointSe := v1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{"240.10.1.1"},
		Ports: []*v1alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      v1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global-west", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
			{Address: "dummy.admiral.global-east", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
		},
	}

	oneEndpointSe := v1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.my-first-service.mesh"},
		Addresses: []string{"240.10.1.1"},
		Ports: []*v1alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      v1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global-west", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
		},
	}

	invalidEndpoint := v1alpha3.ServiceEntry{
		Hosts:     []string{"e2e.test-service.mesh"},
		Addresses: []string{"240.10.1.1"},
		Ports: []*v1alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
			Name: "http", Protocol: "http"}},
		Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
		Resolution:      v1alpha3.ServiceEntry_DNS,
		SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
		Endpoints: []*v1alpha3.WorkloadEntry{
			{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
			{Address: "test.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
		},
	}

	invalidEndpointSe := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se3", Namespace: "namespace"},
		//nolint
		Spec: invalidEndpoint,
	}

	newSeOneEndpoint := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se1", Namespace: "namespace"},
		//nolint
		Spec: oneEndpointSe,
	}

	oldSeTwoEndpoints := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se2", Namespace: "namespace"},
		//nolint
		Spec: twoEndpointSe,
	}

	_, err := seCtrl.IstioClient.NetworkingV1alpha3().ServiceEntries("namespace").Create(ctx, oldSeTwoEndpoints, metaV1.CreateOptions{})
	if err != nil {
		t.Error(err)
	}

	rcWarmupPhase := &RemoteController{
		ServiceEntryController: seCtrl,
		StartTime:              time.Now(),
	}

	rcNotInWarmupPhase := &RemoteController{
		ServiceEntryController: seCtrl,
		StartTime:              time.Now().Add(time.Duration(-21) * time.Minute),
	}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name            string
		rc              *RemoteController
		newSe           *v1alpha32.ServiceEntry
		oldSe           *v1alpha32.ServiceEntry
		skipDestructive bool
		expErr          error
	}{
		{
			name:            "Should add a new SE",
			rc:              rcWarmupPhase,
			newSe:           newSeOneEndpoint,
			oldSe:           nil,
			skipDestructive: false,
		},
		{
			name:            "Should not update SE when in warm up mode and the update is destructive",
			rc:              rcWarmupPhase,
			newSe:           newSeOneEndpoint,
			oldSe:           oldSeTwoEndpoints,
			skipDestructive: true,
		},
		{
			name:            "Should update an SE",
			rc:              rcNotInWarmupPhase,
			newSe:           newSeOneEndpoint,
			oldSe:           oldSeTwoEndpoints,
			skipDestructive: false,
		},
		{
			name:            "Should create an SE with one endpoint",
			rc:              rcNotInWarmupPhase,
			newSe:           invalidEndpointSe,
			oldSe:           nil,
			skipDestructive: false,
		},
		{
			name: "Given serviceentry does not exist, " +
				"And the existing object obtained from Get is nil, " +
				"When another thread create the serviceentry, " +
				"When this thread attempts to create serviceentry and fails, " +
				"Then, then an Update operation should be run, " +
				"And there should be no panic," +
				"And no errors should be returned",
			rc:              rcNotInWarmupPhase,
			newSe:           newSeOneEndpoint,
			oldSe:           nil,
			skipDestructive: false,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := addUpdateServiceEntry(ctxLogger, ctx, c.newSe, c.oldSe, "namespace", c.rc)
			if c.expErr == nil {
				assert.Equal(t, c.expErr, err)
			}
			if c.expErr != nil {
				assert.Equal(t, c.expErr, err)
			}
			if c.skipDestructive {
				//verify the update did not go through
				se, err := c.rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries("namespace").Get(ctx, c.oldSe.Name, metaV1.GetOptions{})
				if err != nil {
					t.Error(err)
				}
				_, diff := getServiceEntryDiff(c.oldSe, se)
				if diff != "" {
					t.Errorf("Failed. Got %v, expected %v", se.Spec.String(), c.oldSe.Spec.String())
				}
			}
		})
	}
}

func TestValidateServiceEntryEndpoints(t *testing.T) {
	twoValidEndpoints := []*v1alpha3.WorkloadEntry{
		{Address: "valid1.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
		{Address: "valid2.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
	}

	oneValidEndpoints := []*v1alpha3.WorkloadEntry{
		{Address: "valid1.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
	}

	dummyEndpoints := []*v1alpha3.WorkloadEntry{
		{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
	}

	validAndInvalidEndpoints := []*v1alpha3.WorkloadEntry{
		{Address: "dummy.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
		{Address: "valid2.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"},
	}

	twoValidEndpointsSe := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "twoValidEndpointsSe", Namespace: "namespace"},
		Spec: v1alpha3.ServiceEntry{
			Hosts:     []string{"e2e.my-first-service.mesh"},
			Addresses: []string{"240.10.1.1"},
			Ports: []*v1alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
				Name: "http", Protocol: "http"}},
			Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
			Resolution:      v1alpha3.ServiceEntry_DNS,
			SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
			Endpoints:       twoValidEndpoints,
		},
	}

	oneValidEndpointsSe := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "twoValidEndpointsSe", Namespace: "namespace"},
		Spec: v1alpha3.ServiceEntry{
			Hosts:     []string{"e2e.my-first-service.mesh"},
			Addresses: []string{"240.10.1.1"},
			Ports: []*v1alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
				Name: "http", Protocol: "http"}},
			Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
			Resolution:      v1alpha3.ServiceEntry_DNS,
			SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
			Endpoints:       oneValidEndpoints,
		},
	}

	dummyEndpointsSe := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "twoValidEndpointsSe", Namespace: "namespace"},
		Spec: v1alpha3.ServiceEntry{
			Hosts:     []string{"e2e.my-first-service.mesh"},
			Addresses: []string{"240.10.1.1"},
			Ports: []*v1alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
				Name: "http", Protocol: "http"}},
			Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
			Resolution:      v1alpha3.ServiceEntry_DNS,
			SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
			Endpoints:       dummyEndpoints,
		},
	}

	validAndInvalidEndpointsSe := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "twoValidEndpointsSe", Namespace: "namespace"},
		Spec: v1alpha3.ServiceEntry{
			Hosts:     []string{"e2e.my-first-service.mesh"},
			Addresses: []string{"240.10.1.1"},
			Ports: []*v1alpha3.ServicePort{{Number: uint32(common.DefaultServiceEntryPort),
				Name: "http", Protocol: "http"}},
			Location:        v1alpha3.ServiceEntry_MESH_INTERNAL,
			Resolution:      v1alpha3.ServiceEntry_DNS,
			SubjectAltNames: []string{"spiffe://prefix/my-first-service"},
			Endpoints:       validAndInvalidEndpoints,
		},
	}

	//Struct of test case info. Name is required.
	testCases := []struct {
		name                      string
		serviceEntry              *v1alpha32.ServiceEntry
		expectedAreEndpointsValid bool
		expectedValidEndpoints    []*v1alpha3.WorkloadEntry
	}{
		{
			name:                      "Validate SE with dummy endpoint",
			serviceEntry:              dummyEndpointsSe,
			expectedAreEndpointsValid: false,
			expectedValidEndpoints:    []*v1alpha3.WorkloadEntry{},
		},
		{
			name:                      "Validate SE with valid endpoint",
			serviceEntry:              oneValidEndpointsSe,
			expectedAreEndpointsValid: true,
			expectedValidEndpoints:    []*v1alpha3.WorkloadEntry{{Address: "valid1.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"}},
		},
		{
			name:                      "Validate endpoint with multiple valid endpoints",
			serviceEntry:              twoValidEndpointsSe,
			expectedAreEndpointsValid: true,
			expectedValidEndpoints: []*v1alpha3.WorkloadEntry{
				{Address: "valid1.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-west-2"},
				{Address: "valid2.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"}},
		},
		{
			name:                      "Validate endpoint with mix of valid and dummy endpoints",
			serviceEntry:              validAndInvalidEndpointsSe,
			expectedAreEndpointsValid: false,
			expectedValidEndpoints: []*v1alpha3.WorkloadEntry{
				{Address: "valid2.admiral.global", Ports: map[string]uint32{"http": 0}, Locality: "us-east-2"}},
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			areValidEndpoints := validateAndProcessServiceEntryEndpoints(c.serviceEntry)
			if areValidEndpoints != c.expectedAreEndpointsValid {
				t.Errorf("Failed. Got %v, expected %v", areValidEndpoints, c.expectedAreEndpointsValid)
			}
			if len(c.serviceEntry.Spec.Endpoints) != len(c.expectedValidEndpoints) {
				t.Errorf("Failed. Got %v, expected %v", len(c.serviceEntry.Spec.Endpoints), len(c.expectedValidEndpoints))
			}
		})
	}
}

func TestServiceEntryHandlerCUDScenarios(t *testing.T) {
	admiralParams := common.AdmiralParams{
		LabelSet:            &common.LabelSet{},
		SyncNamespace:       "test-sync-ns",
		ArgoRolloutsEnabled: true,
	}
	common.InitializeConfig(admiralParams)
	se := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: "istio-system",
			Name:      "test-serviceentry",
			Annotations: map[string]string{
				"admiral.istio.io/ignore": "true",
			},
		},
		Spec: v1alpha3.ServiceEntry{
			Hosts: []string{"test-host"},
			Ports: []*v1alpha3.ServicePort{
				{
					Number:   80,
					Protocol: "TCP",
				},
			},
			Location: v1alpha3.ServiceEntry_MESH_INTERNAL,
		},
	}
	seHandler := &ServiceEntryHandler{
		ClusterID: "test-cluster",
	}

	testcases := []struct {
		name             string
		admiralReadState bool
		ns               string
	}{
		{
			name:             "Admiral in read-only state",
			admiralReadState: true,
			ns:               "test-ns",
		},
		{
			name:             "Encountered istio resource",
			admiralReadState: false,
			ns:               "istio-system",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			commonUtil.CurrentAdmiralState.ReadOnly = tc.admiralReadState
			se.ObjectMeta.Namespace = tc.ns
			err := seHandler.Added(se)
			assert.NoError(t, err)
			err = seHandler.Updated(se)
			assert.NoError(t, err)
			err = seHandler.Deleted(se)
			assert.NoError(t, err)
		})
	}
}

func TestAddServiceEntry(t *testing.T) {
	ctxLogger := log.WithFields(log.Fields{
		"type": "modifySE",
	})
	se := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{Name: "se1", Namespace: "random"},
	}
	admiralParams := common.AdmiralParams{
		LabelSet:      &common.LabelSet{},
		SyncNamespace: "test-sync-ns",
	}
	common.InitializeConfig(admiralParams)
	ctx := context.Background()
	rc := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: istioFake.NewSimpleClientset(),
		},
	}
	err := deleteServiceEntry(ctx, se, admiralParams.SyncNamespace, rc)
	assert.Nil(t, err)
	addUpdateServiceEntry(ctxLogger, ctx, se, nil, admiralParams.SyncNamespace, rc)
	assert.Nil(t, err)
	err = deleteServiceEntry(ctx, se, admiralParams.SyncNamespace, rc)
	assert.Nil(t, err)
}

func TestRetryUpdatingSE(t *testing.T) {
	// Create a mock logger
	logger := log.New()
	admiralParams := common.AdmiralParams{
		LabelSet:      &common.LabelSet{},
		SyncNamespace: "test-sync-ns",
	}
	common.ResetSync()
	common.InitializeConfig(admiralParams)
	//Create a context with timeout for testing
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	admiralParams = common.GetAdmiralParams()
	log.Info("admiralSyncNS: " + admiralParams.SyncNamespace)
	// Create mock objects
	obj := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: admiralParams.SyncNamespace,
			Name:      "test-serviceentry-seRetriesTest",
			Annotations: map[string]string{
				"admiral.istio.io/ignore": "true",
			},
			ResourceVersion: "123",
		},
		Spec: v1alpha3.ServiceEntry{
			Hosts: []string{"test-host"},
			Ports: []*v1alpha3.ServicePort{
				{
					Number:   80,
					Protocol: "TCP",
				},
			},
			Location: v1alpha3.ServiceEntry_MESH_INTERNAL,
		},
	}
	exist := &v1alpha32.ServiceEntry{
		ObjectMeta: metaV1.ObjectMeta{
			Namespace: admiralParams.SyncNamespace,
			Name:      "test-serviceentry-seRetriesTest",
			Annotations: map[string]string{
				"admiral.istio.io/ignore": "true",
			},
			ResourceVersion: "12345",
		},
		Spec: v1alpha3.ServiceEntry{
			Hosts: []string{"test-host"},
			Ports: []*v1alpha3.ServicePort{
				{
					Number:   80,
					Protocol: "TCP",
				},
			},
			Location: v1alpha3.ServiceEntry_MESH_INTERNAL,
		},
	}
	namespace := admiralParams.SyncNamespace
	rc := &RemoteController{
		ServiceEntryController: &istio.ServiceEntryController{
			IstioClient: istioFake.NewSimpleClientset(),
		},
	}

	_, err := rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Create(ctx, exist, metaV1.CreateOptions{})
	if err != nil {
		t.Error(err)
	}
	obj2, err2 := rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Create(ctx, exist, metaV1.CreateOptions{})
	if k8sErrors.IsAlreadyExists(err2) {
		fmt.Printf("obj: %v", obj2)
	}
	errConflict := k8sErrors.NewConflict(schema.GroupResource{}, "", nil)
	errOther := errors.New("Some other error")

	// Test when err is nil
	err = retryUpdatingSE(logger.WithField("test", "success"), ctx, obj, exist, namespace, rc, nil, "test-op")
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}

	// get the SE here, it should still have the old resource version.
	se, err := rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(namespace).Get(ctx, exist.Name, metaV1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "12345", se.ObjectMeta.ResourceVersion)

	// Test when err is a conflict error
	err = retryUpdatingSE(logger.WithField("test", "conflict"), ctx, obj, exist, namespace, rc, errConflict, "test-op")
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}

	// get the SE and the resourceVersion should have been updated to 12345
	se, err = rc.ServiceEntryController.IstioClient.NetworkingV1alpha3().ServiceEntries(admiralParams.SyncNamespace).Get(ctx, exist.Name, metaV1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "12345", se.ObjectMeta.ResourceVersion)

	// Test when err is a non-conflict error
	err = retryUpdatingSE(logger.WithField("test", "error"), ctx, obj, exist, namespace, rc, errOther, "test-op")
	if err == nil {
		t.Error("Expected non-nil error, got nil")
	}
}
