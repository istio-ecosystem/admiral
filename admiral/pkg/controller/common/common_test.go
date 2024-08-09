package common

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"

	"github.com/stretchr/testify/assert"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	admiralv1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	v12 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sCoreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//v1admiral "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
)

var ignoreUnexported = cmpopts.IgnoreUnexported(v12.GlobalTrafficPolicy{}.Status)

func init() {
	initConfig(false, false)
}

func initConfig(fqdn bool, fqdnLocal bool) {
	ResetSync()
	p := AdmiralParams{
		KubeconfigPath:                      "testdata/fake.config",
		LabelSet:                            &LabelSet{},
		EnableSAN:                           true,
		SANPrefix:                           "prefix",
		HostnameSuffix:                      "mesh",
		SyncNamespace:                       "ns",
		CacheReconcileDuration:              time.Minute,
		ClusterRegistriesNamespace:          "default",
		DependenciesNamespace:               "default",
		WorkloadSidecarName:                 "default",
		WorkloadSidecarUpdate:               "disabled",
		MetricsEnabled:                      true,
		EnableRoutingPolicy:                 true,
		EnvoyFilterVersion:                  "1.13",
		EnableAbsoluteFQDN:                  fqdn,
		EnableAbsoluteFQDNForLocalEndpoints: fqdnLocal,
		EnableSWAwareNSCaches:               true,
		ExportToIdentityList:                []string{"*"},
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.AdmiralCRDIdentityLabel = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.IdentityPartitionKey = "admiral.io/identityPartition"
	InitializeConfig(p)
}

func TestGetTrafficConfigTransactionID(t *testing.T) {
	tc := admiralv1.TrafficConfig{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"transactionID": ""},
			Annotations: map[string]string{
				"transactionID": "123456",
			}},
	}
	tid := GetTrafficConfigTransactionID(&tc)
	assert.NotNil(t, tid)
}

func TestGetTrafficConfigRevision(t *testing.T) {
	tc := admiralv1.TrafficConfig{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"revisionNumber": ""},
			Annotations: map[string]string{
				"revisionNumber": "123456",
			}},
	}
	tid := GetTrafficConfigRevision(&tc)
	assert.NotNil(t, tid)
}

func TestGetTrafficConfigIdentity(t *testing.T) {
	tc := admiralv1.TrafficConfig{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"asset": ""},
			Annotations: map[string]string{
				"asset": "123456",
			}},
	}
	tid := GetTrafficConfigIdentity(&tc)
	assert.NotNil(t, tid)
}

func TestGetSAN(t *testing.T) {
	t.Parallel()

	identifier := "identity"
	identifierVal := "company.platform.server"
	domain := "preprd"

	deployment := k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{identifier: identifierVal}}}}}
	deploymentWithAnnotation := k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal}}}}}

	deploymentWithNoIdentifier := k8sAppsV1.Deployment{}

	testCases := []struct {
		name       string
		deployment k8sAppsV1.Deployment
		domain     string
		wantSAN    string
	}{
		{
			name:       "should return valid SAN (from label)",
			deployment: deployment,
			domain:     domain,
			wantSAN:    "spiffe://" + domain + "/" + identifierVal,
		},
		{
			name:       "should return valid SAN (from annotation)",
			deployment: deploymentWithAnnotation,
			domain:     domain,
			wantSAN:    "spiffe://" + domain + "/" + identifierVal,
		},
		{
			name:       "should return valid SAN with no domain prefix",
			deployment: deployment,
			domain:     "",
			wantSAN:    "spiffe://" + identifierVal,
		},
		{
			name:       "should return empty SAN",
			deployment: deploymentWithNoIdentifier,
			domain:     domain,
			wantSAN:    "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			san := GetSAN(c.domain, &c.deployment, identifier)
			if !reflect.DeepEqual(san, c.wantSAN) {
				t.Errorf("Wanted SAN: %s, got: %s", c.wantSAN, san)
			}
		})
	}

}

func TestGetCname(t *testing.T) {

	nameSuffix := "global"
	identifier := "identity"
	identifierVal := "COMPANY.platform.server"

	testCases := []struct {
		name       string
		deployment k8sAppsV1.Deployment
		expected   string
	}{
		{
			name:       "should return valid cname (from label)",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   strings.ToLower("stage." + identifierVal + ".global"),
		}, {
			name:       "should return valid cname (from label) uses case sensitive DNS annotation -enabled",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"admiral.io/cname-case-sensitive": "true"}, Labels: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   "stage." + identifierVal + ".global",
		}, {
			name:       "should return valid cname (from label)  uses case sensitive DNS annotation -disabled",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"admiral.io/cname-case-sensitive": "false"}, Labels: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   strings.ToLower("stage." + identifierVal + ".global"),
		},
		{
			name:       "should return valid cname (from annotation)",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal}, Labels: map[string]string{"env": "stage"}}}}},
			expected:   strings.ToLower("stage." + identifierVal + ".global"),
		},
		{
			name:       "should return empty string",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{"env": "stage"}}}}},
			expected:   "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			cname := GetCname(&c.deployment, identifier, nameSuffix)
			if !(cname == c.expected) {
				t.Errorf("Wanted Cname: %s, got: %s", c.expected, cname)
			}
		})
	}
}

func TestGetLocalDomainSuffix(t *testing.T) {

	testCases := []struct {
		name                string
		FQDNEnabled         bool
		FQDNEnabledForLocal bool
		expected            string
	}{
		{
			name:                "should return .local endpoint suffix, when FQDN is disabled and ForLocal is disabled",
			FQDNEnabled:         false,
			FQDNEnabledForLocal: false,
			expected:            ".svc.cluster.local",
		},
		{
			name:                "should return .local endpoint suffix, when FQDN is enabled and ForLocal is disabled",
			FQDNEnabled:         true,
			FQDNEnabledForLocal: false,
			expected:            ".svc.cluster.local",
		},
		{
			name:                "should return .local endpoint suffix, when FQDN is disabled and ForLocal is enabled",
			FQDNEnabled:         false,
			FQDNEnabledForLocal: true,
			expected:            ".svc.cluster.local",
		},
		{
			name:                "should return .local. endpoint suffix, when FQDN is enabled and ForLocal is enabled",
			FQDNEnabled:         true,
			FQDNEnabledForLocal: true,
			expected:            ".svc.cluster.local.",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			initConfig(c.FQDNEnabled, c.FQDNEnabledForLocal)
			suffix := GetLocalDomainSuffix()
			if !(suffix == c.expected) {
				t.Errorf("Wanted suffix: %s, got: %s", c.expected, suffix)
			}
		})
	}
}

func TestNodeLocality(t *testing.T) {

	nodeLocalityLabel := "us-west-2"

	testCases := []struct {
		name     string
		node     k8sCoreV1.Node
		expected string
	}{
		{
			name:     "should return valid node region",
			node:     k8sCoreV1.Node{Spec: k8sCoreV1.NodeSpec{}, ObjectMeta: v1.ObjectMeta{Labels: map[string]string{NodeRegionLabel: nodeLocalityLabel}}},
			expected: nodeLocalityLabel,
		},
		{
			name:     "should return empty value when node annotation isn't present",
			node:     k8sCoreV1.Node{Spec: k8sCoreV1.NodeSpec{}, ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{}}},
			expected: "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			locality := GetNodeLocality(&c.node)
			if !(locality == c.expected) {
				t.Errorf("Wanted locality: %s, got: %s", c.expected, locality)
			}
		})
	}
}

func TestGetDeploymentGlobalIdentifier(t *testing.T) {
	initConfig(true, true)
	identifier := "identity"
	identifierVal := "company.platform.server"

	testCases := []struct {
		name       string
		deployment k8sAppsV1.Deployment
		expected   string
		originalex string
	}{
		{
			name:       "should return valid identifier from label",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   identifierVal,
			originalex: identifierVal,
		},
		{
			name:       "should return valid identifier from annotations",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   identifierVal,
			originalex: identifierVal,
		},
		{
			name:       "should return partitioned identifier",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal, "env": "stage", "admiral.io/identityPartition": "pid"}}}}},
			expected:   "pid." + identifierVal,
			originalex: identifierVal,
		},
		{
			name:       "should return empty identifier",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}, Annotations: map[string]string{}}}}},
			expected:   "",
			originalex: "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			iVal := GetDeploymentGlobalIdentifier(&c.deployment)
			if !(iVal == c.expected) {
				t.Errorf("Wanted identity value: %s, got: %s", c.expected, iVal)
			}
			oiVal := GetDeploymentOriginalIdentifier(&c.deployment)
			if !(oiVal == c.originalex) {
				t.Errorf("Wanted original identity value: %s, got: %s", c.originalex, oiVal)
			}
		})
	}
}

func TestGetDeploymentIdentityPartition(t *testing.T) {
	initConfig(true, true)
	partitionIdentifier := "admiral.io/identityPartition"
	identifierVal := "swX"

	testCases := []struct {
		name       string
		deployment k8sAppsV1.Deployment
		expected   string
	}{
		{
			name:       "should return valid identifier from label",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{partitionIdentifier: identifierVal, "env": "stage"}}}}},
			expected:   identifierVal,
		},
		{
			name:       "should return valid identifier from annotations",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{partitionIdentifier: identifierVal, "env": "stage"}}}}},
			expected:   identifierVal,
		},
		{
			name:       "should return empty identifier",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}, Annotations: map[string]string{}}}}},
			expected:   "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			iVal := GetDeploymentIdentityPartition(&c.deployment)
			if !(iVal == c.expected) {
				t.Errorf("Wanted identity partition value: %s, got: %s", c.expected, iVal)
			}
		})
	}
}

func TestGetPodGlobalIdentifier(t *testing.T) {

	identifier := "identity"
	identifierVal := "company.platform.server"

	testCases := []struct {
		name     string
		pod      k8sCoreV1.Pod
		expected string
	}{
		{
			name:     "should return valid identifier from label",
			pod:      k8sCoreV1.Pod{Spec: k8sCoreV1.PodSpec{}, ObjectMeta: v1.ObjectMeta{Labels: map[string]string{identifier: identifierVal, "env": "stage"}}},
			expected: identifierVal,
		},
		{
			name:     "should return valid identifier from annotation",
			pod:      k8sCoreV1.Pod{Spec: k8sCoreV1.PodSpec{}, ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal, "env": "stage"}}},
			expected: identifierVal,
		},
		{
			name:     "should return empty identifier",
			pod:      k8sCoreV1.Pod{Spec: k8sCoreV1.PodSpec{}, ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{}, Labels: map[string]string{}}},
			expected: "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			iVal := GetPodGlobalIdentifier(&c.pod)
			if !(iVal == c.expected) {
				t.Errorf("Wanted identity value: %s, got: %s", c.expected, iVal)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {

	testCases := []struct {
		name       string
		deployment k8sAppsV1.Deployment
		expected   string
	}{
		{
			name:       "should return default env",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}}}},
			expected:   Default,
		},
		{
			name:       "should return valid env from label",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{}, Labels: map[string]string{"env": "stage2"}}}}},
			expected:   "stage2",
		},
		{
			name:       "should return valid env from new annotation",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"admiral.io/env": "stage1"}, Labels: map[string]string{"env": "stage2"}}}}},
			expected:   "stage1",
		},
		{
			name:       "should return valid env from new label",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{}, Labels: map[string]string{"admiral.io/env": "production", "env": "stage2"}}}}},
			expected:   "production",
		},
		{
			name:       "should return env from namespace suffix",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}}}, ObjectMeta: v1.ObjectMeta{Namespace: "uswest2-prd"}},
			expected:   "prd",
		},
		{
			name:       "should return default when namespace doesn't have blah..region-env format",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}}}, ObjectMeta: v1.ObjectMeta{Namespace: "sample"}},
			expected:   Default,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			env := GetEnv(&c.deployment)
			if !(env == c.expected) {
				t.Errorf("Wanted Cname: %s, got: %s", c.expected, env)
			}
		})
	}
}

func TestGetGtpEnv(t *testing.T) {

	envNewAnnotationGtp := v12.GlobalTrafficPolicy{}
	envNewAnnotationGtp.CreationTimestamp = v1.Now()
	envNewAnnotationGtp.Labels = map[string]string{"identity": "app1", "admiral.io/env": "stage1"}
	envNewAnnotationGtp.Annotations = map[string]string{"admiral.io/env": "production"}
	envNewAnnotationGtp.Namespace = "namespace"
	envNewAnnotationGtp.Name = "myGTP-new-annotation"

	envNewLabelGtp := v12.GlobalTrafficPolicy{}
	envNewLabelGtp.CreationTimestamp = v1.Now()
	envNewLabelGtp.Labels = map[string]string{"identity": "app1", "admiral.io/env": "stage1", "env": "stage2"}
	envNewLabelGtp.Namespace = "namespace"
	envNewLabelGtp.Name = "myGTP-new-label"

	envLabelGtp := v12.GlobalTrafficPolicy{}
	envLabelGtp.CreationTimestamp = v1.Now()
	envLabelGtp.Labels = map[string]string{"identity": "app1", "env": "stage2"}
	envLabelGtp.Namespace = "namespace"
	envLabelGtp.Name = "myGTP-label"

	noEnvGtp := v12.GlobalTrafficPolicy{}
	noEnvGtp.CreationTimestamp = v1.Now()
	noEnvGtp.Labels = map[string]string{"identity": "app1"}
	noEnvGtp.Namespace = "namespace"
	noEnvGtp.Name = "myGTP-no-env"

	testCases := []struct {
		name        string
		gtp         *v12.GlobalTrafficPolicy
		expectedEnv string
	}{
		{
			name:        "Should return env from new annotation",
			gtp:         &envNewAnnotationGtp,
			expectedEnv: "production",
		},
		{
			name:        "Should return env from new label",
			gtp:         &envNewLabelGtp,
			expectedEnv: "stage1",
		},
		{
			name:        "Should return env from label",
			gtp:         &envLabelGtp,
			expectedEnv: "stage2",
		},
		{
			name:        "Should return default with no env specified",
			gtp:         &noEnvGtp,
			expectedEnv: "default",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			returned := GetGtpEnv(c.gtp)
			if !cmp.Equal(returned, c.expectedEnv, ignoreUnexported) {
				t.Fatalf("GTP env mismatch. Diff: %v", cmp.Diff(returned, c.expectedEnv, ignoreUnexported))
			}
		})
	}

}

func TestGetRoutingPolicyEnv(t *testing.T) {

	envNewAnnotationRP := v12.RoutingPolicy{}
	envNewAnnotationRP.CreationTimestamp = v1.Now()
	envNewAnnotationRP.Labels = map[string]string{"identity": "app1", "admiral.io/env": "stage1"}
	envNewAnnotationRP.Annotations = map[string]string{"identity": "app1", "admiral.io/env": "stage1"}
	envNewAnnotationRP.Namespace = "namespace"
	envNewAnnotationRP.Name = "myRP-new-annotation"

	envLabelRP := v12.RoutingPolicy{}
	envLabelRP.CreationTimestamp = v1.Now()
	envLabelRP.Labels = map[string]string{"admiral.io/env": "stage1", "env": "stage2"}
	envLabelRP.Namespace = "namespace"
	envLabelRP.Name = "myRP-label"

	noEnvRP := v12.RoutingPolicy{}
	noEnvRP.CreationTimestamp = v1.Now()
	noEnvRP.Namespace = "namespace"
	noEnvRP.Name = "myRP-no-env"

	testCases := []struct {
		name        string
		rp          *v12.RoutingPolicy
		expectedEnv string
	}{
		{
			name:        "Should return env from new annotation",
			rp:          &envNewAnnotationRP,
			expectedEnv: "stage1",
		},
		{
			name:        "Should return env from new label",
			rp:          &envLabelRP,
			expectedEnv: "stage1",
		},
		{
			name:        "Should return default with no env specified",
			rp:          &noEnvRP,
			expectedEnv: "default",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			returned := GetRoutingPolicyEnv(c.rp)
			if !cmp.Equal(returned, c.expectedEnv, ignoreUnexported) {
				t.Fatalf("RP env mismatch. Diff: %v", cmp.Diff(returned, c.expectedEnv, ignoreUnexported))
			}
		})
	}

}

func TestGetGtpIdentity(t *testing.T) {

	gtpIdentityFromLabels := v12.GlobalTrafficPolicy{}
	gtpIdentityFromLabels.CreationTimestamp = v1.Now()
	gtpIdentityFromLabels.Labels = map[string]string{"identity": "app1", "admiral.io/env": "stage1"}
	gtpIdentityFromLabels.Annotations = map[string]string{"admiral.io/env": "production"}
	gtpIdentityFromLabels.Namespace = "namespace"
	gtpIdentityFromLabels.Name = "myGTP"

	gtpIdenityFromSelector := v12.GlobalTrafficPolicy{}
	gtpIdenityFromSelector.CreationTimestamp = v1.Now()
	gtpIdenityFromSelector.Labels = map[string]string{"admiral.io/env": "stage1", "env": "stage2"}
	gtpIdenityFromSelector.Spec.Selector = map[string]string{"identity": "app2", "admiral.io/env": "stage1", "env": "stage2"}
	gtpIdenityFromSelector.Namespace = "namespace"
	gtpIdenityFromSelector.Name = "myGTP"

	testCases := []struct {
		name             string
		gtp              *v12.GlobalTrafficPolicy
		expectedIdentity string
	}{
		{
			name:             "Should return the identity from the labels",
			gtp:              &gtpIdentityFromLabels,
			expectedIdentity: "app1",
		},
		{
			name:             "Should return the identity from the selector",
			gtp:              &gtpIdenityFromSelector,
			expectedIdentity: "app2",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			returned := GetGtpIdentity(c.gtp)
			if !cmp.Equal(returned, c.expectedIdentity, ignoreUnexported) {
				t.Fatalf("GTP identity mismatch. Diff: %v", cmp.Diff(returned, c.expectedIdentity, ignoreUnexported))
			}
		})
	}

}

func TestIsServiceMatch(t *testing.T) {
	matchingSelector := v1.LabelSelector{}
	matchingSelector.MatchLabels = map[string]string{"app": "app1", "asset": "asset1"}
	matchingServiceSelector := map[string]string{"app": "app1", "asset": "asset1"}

	nonMatchingSelector := v1.LabelSelector{}
	nonMatchingSelector.MatchLabels = map[string]string{"app": "app1", "asset": "asset1"}
	nonMatchingServiceSelector := map[string]string{"app": "app2", "asset": "asset1"}

	nilSelector := v1.LabelSelector{}
	nonNilServiceSelector := map[string]string{"app": "app1", "asset": "asset1"}

	nonNilSelector := v1.LabelSelector{}
	nonNilSelector.MatchLabels = map[string]string{"app": "app1", "asset": "asset1"}
	nilServiceSelector := map[string]string{}
	testCases := []struct {
		name            string
		selector        *v1.LabelSelector
		serviceSelector map[string]string
		expectedBool    bool
	}{
		{
			name:            "service selector and selector matches",
			selector:        &matchingSelector,
			serviceSelector: matchingServiceSelector,
			expectedBool:    true,
		},
		{
			name:            "service selector and selector do not match",
			selector:        &nonMatchingSelector,
			serviceSelector: nonMatchingServiceSelector,
			expectedBool:    false,
		},
		{
			name:            "selector is nil",
			selector:        &nilSelector,
			serviceSelector: nonNilServiceSelector,
			expectedBool:    false,
		},
		{
			name:            "service selector is nil",
			selector:        &nonNilSelector,
			serviceSelector: nilServiceSelector,
			expectedBool:    false,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			actualBool := IsServiceMatch(c.serviceSelector, c.selector)
			if actualBool != c.expectedBool {
				t.Fatalf("Failed. Expected: %t, Got: %t", c.expectedBool, actualBool)
			}
		})
	}
}

func TestGetRoutingPolicyIdentity(t *testing.T) {
	rp := &admiralv1.RoutingPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"routingPolicy": "test-policy",
				"identity":      "mock-identity",
			},
		},
	}

	identity := GetRoutingPolicyIdentity(rp)
	expected := "mock-identity"
	if identity != expected {
		t.Errorf("Expected identity to be %s, but got %s", expected, identity)
	}
}

func TestGetRoutingPolicy(t *testing.T) {
	testcases := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{
			name: "When Env Label and Annotation Not Set",
			labels: map[string]string{
				"routingPolicy": "test-policy",
				"identity":      "mock-identity",
			},
			expected: Default + ".mock-identity",
		},
		{
			name: "When Env Label Set",
			labels: map[string]string{
				"routingPolicy":  "test-policy",
				"admiral.io/env": "test-env",
				"identity":       "mock-identity",
			},
			expected: "test-env.mock-identity",
		},
		{
			name:     "When ObjectMeta.Labels nil",
			labels:   nil,
			expected: Default + ".",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			rp := &admiralv1.RoutingPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Labels:    tc.labels,
				},
			}
			key := GetRoutingPolicyKey(rp)
			assert.Equal(t, tc.expected, key)
		})
	}
}

func TestConstructRoutingPolicyKey(t *testing.T) {
	key := ConstructRoutingPolicyKey("test-env", "test-policy")
	expected := "test-env.test-policy"
	if key != expected {
		t.Errorf("Expected key to be %s, but got %s", expected, key)
	}
}

func TestGetSha1(t *testing.T) {
	key := "test-key"

	sha1, err := GetSha1(key)
	if err != nil {
		t.Errorf("Error calling GetSha1: %v", err)
	}
	expectedSha1 := "e22eadd25b24b165d55d"
	if sha1 != expectedSha1 {
		t.Errorf("Expected SHA1 to be %s, but got %s", expectedSha1, sha1)
	}
}

func TestGetBytes(t *testing.T) {
	key := "test-key"
	bv, err := GetBytes(key)
	if err != nil {
		t.Errorf("Error calling GetBytes: %v", err)
	}
	expectedbytes := []byte("test-key")

	bv = bv[len(bv)-len(expectedbytes):]
	if !bytes.Equal(bv, expectedbytes) {
		t.Errorf("Expected bytes to be %v, but got %v", expectedbytes, bv)
	}
}

func TestAppendError(t *testing.T) {
	var err error

	err = AppendError(err, err)
	assert.Nil(t, err)

	errNew := errors.New("test error 1")
	err = AppendError(err, errNew)

	assert.Equal(t, errNew.Error(), err.Error())

	errNew2 := errors.New("test error 2")
	err = AppendError(err, errNew2)

	assert.Equal(t, errNew.Error()+"; "+errNew2.Error(), err.Error())
}

func TestGetODIdentity(t *testing.T) {
	type args struct {
		od *admiralv1.OutlierDetection
	}

	test1od := &admiralv1.OutlierDetection{
		TypeMeta:   v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{},
		Spec:       model.OutlierDetection{},
		Status:     v12.OutlierDetectionStatus{},
	}
	test1od.Labels = make(map[string]string)
	test1od.Labels["identity"] = "foo"

	test2od := &admiralv1.OutlierDetection{
		TypeMeta:   v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{},
		Spec:       model.OutlierDetection{},
		Status:     v12.OutlierDetectionStatus{},
	}
	test2od.Labels = make(map[string]string)
	tests := []struct {
		name string
		args args
		want string
	}{
		{"Get Identity as foo", args{od: test1od}, "foo"},
		{"Get Identity as empty", args{od: test2od}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, GetODIdentity(tt.args.od), "GetODIdentity(%v)", tt.args.od)
		})
	}
}

func TestGetODEnv(t *testing.T) {
	type args struct {
		od *admiralv1.OutlierDetection
	}

	test1od := &admiralv1.OutlierDetection{
		TypeMeta:   v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{},
		Spec:       model.OutlierDetection{},
		Status:     v12.OutlierDetectionStatus{},
	}
	test1od.Labels = make(map[string]string)

	test2od := &admiralv1.OutlierDetection{
		TypeMeta:   v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{},
		Spec:       model.OutlierDetection{},
		Status:     v12.OutlierDetectionStatus{},
	}
	test2od.Annotations = make(map[string]string)
	test2od.Annotations["admiral.io/env"] = "fooAnnotation"

	test3od := &admiralv1.OutlierDetection{
		TypeMeta:   v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{},
		Spec:       model.OutlierDetection{},
		Status:     v12.OutlierDetectionStatus{},
	}
	test3od.Labels = make(map[string]string)
	test3od.Labels["admiral.io/env"] = "fooLabel"

	test4od := &admiralv1.OutlierDetection{
		TypeMeta:   v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{},
		Spec:       model.OutlierDetection{},
		Status:     v12.OutlierDetectionStatus{},
	}

	selector := make(map[string]string)
	selector["admiral.io/env"] = "fooSelector"

	test4od.Spec = model.OutlierDetection{
		Selector: selector,
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"NotEnvAdded", args{od: test1od}, Default},
		{"EnvAddedAtAnnotation", args{od: test2od}, "fooAnnotation"},
		{"EnvAddedAtLabel", args{od: test3od}, "fooLabel"},
		{"EnvAddedAtLabelSelector", args{od: test4od}, "fooSelector"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, GetODEnv(tt.args.od), "GetODEnv(%v)", tt.args.od)
		})
	}
}

func TestCheckIFEnvLabelIsPresentEnvValueEmpty(t *testing.T) {
	tc := admiralv1.TrafficConfig{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"env": ""},
			Annotations: map[string]string{
				"asset": "123456",
			}},
	}
	tid := CheckIFEnvLabelIsPresent(&tc)
	assert.NotNil(t, tid)
}

func TestCheckIFEnvLabelIsPresentLabelsMissing(t *testing.T) {
	tc := admiralv1.TrafficConfig{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"asset": "123456",
			}},
	}
	tid := CheckIFEnvLabelIsPresent(&tc)
	assert.NotNil(t, tid)
}

func TestCheckIFEnvLabelIsPresentSuccess(t *testing.T) {
	tc := admiralv1.TrafficConfig{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"env": "qal"},
			Annotations: map[string]string{
				"asset": "123456",
			}},
	}
	tid := CheckIFEnvLabelIsPresent(&tc)
	assert.Nil(t, tid)
	ctx := context.Background()

	t.Run("Callback returns nil on first try", func(t *testing.T) {
		callback := func() error {
			return nil
		}
		err := RetryWithBackOff(ctx, callback, 3)
		if err != nil {
			t.Errorf("Expected nil error, but received %v", err)
		}
	})

	t.Run("Callback returns error on every try", func(t *testing.T) {
		callback := func() error {
			return errors.New("some error")
		}
		startTime := time.Now()
		err := RetryWithBackOff(ctx, callback, 3)
		timeTaken := time.Since(startTime)
		expectedTime := 10*time.Second + 20*time.Second
		if err == nil {
			t.Error("Expected error, but received nil")
		}
		if timeTaken < expectedTime {
			t.Errorf("Expected function to run for at least %v, but only ran for %v", expectedTime, timeTaken)
		}
	})

	t.Run("Callback returns error on first try and then nil", func(t *testing.T) {
		static := true
		callback := func() error {
			if static {
				static = false
				return errors.New("some error")
			}
			return nil
		}
		err := RetryWithBackOff(ctx, callback, 3)
		if err != nil {
			t.Errorf("Expected nil error, but received %v", err)
		}
	})
}

func TestGenerateTxId(t *testing.T) {
	type args struct {
		meta     v1.Object
		ctrlName string
		id       string
	}

	testMeta1 := &metav1.ObjectMeta{
		ResourceVersion: "marvel",
		Annotations: map[string]string{
			IntuitTID: "ironman",
		},
	}

	testMeta2 := &metav1.ObjectMeta{
		ResourceVersion: "marvel",
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"When controller is GTP " +
			"And both Resourse Version and Annotation present " +
			"Expect 3 ids <resourceVersion>-<annotationId>-UUID", args{
			meta:     testMeta1,
			ctrlName: GTPCtrl,
			id:       "intuit",
		},
			"marvel-ironman-intuit"},
		{"When controller is empty/non GTP" +
			"And Both resource version and Annotion present " +
			"Expect 2 ids <resource version>-UUID", args{
			meta:     testMeta1,
			ctrlName: "",
			id:       "intuit",
		},
			"marvel-intuit"},
		{"When meta object is nil or not matching, and it is not GTP controller" +
			"Expect 1 ids UUID", args{
			meta:     nil,
			ctrlName: "",
			id:       "intuit",
		},
			"intuit"},
		{"When tid is not present in annotation " +
			"And resourse version present " +
			"Expect 2 tid <resource version>-UUID", args{
			meta:     testMeta2,
			ctrlName: GTPCtrl,
			id:       "intuit",
		},
			"marvel-intuit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, GenerateTxId(tt.args.meta, tt.args.ctrlName, tt.args.id), "GenerateTxId(%v, %v, %v)", tt.args.meta, tt.args.ctrlName, tt.args.id)
		})
	}
}

func TestGetGtpIdentityPartition(t *testing.T) {
	initConfig(true, true)
	partitionIdentifier := "admiral.io/identityPartition"
	identifierVal := "swX"

	testCases := []struct {
		name     string
		gtp      v12.GlobalTrafficPolicy
		expected string
	}{
		{
			name:     "should return valid identifier from label",
			gtp:      v12.GlobalTrafficPolicy{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{partitionIdentifier: identifierVal, "env": "stage"}}},
			expected: identifierVal,
		},
		{
			name:     "should return valid identifier from annotations",
			gtp:      v12.GlobalTrafficPolicy{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{partitionIdentifier: identifierVal, "env": "stage"}}},
			expected: identifierVal,
		},
		{
			name:     "should return empty identifier",
			gtp:      v12.GlobalTrafficPolicy{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}, Annotations: map[string]string{}}},
			expected: "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			iVal := GetGtpIdentityPartition(&c.gtp)
			if !(iVal == c.expected) {
				t.Errorf("Wanted identity partition value: %s, got: %s", c.expected, iVal)
			}
		})
	}
}
