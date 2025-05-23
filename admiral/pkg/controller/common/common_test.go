package common

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/stretchr/testify/assert"
	v1alpha4 "istio.io/client-go/pkg/apis/networking/v1alpha3"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	admiralV1Alpha1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1alpha1"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sCoreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var ignoreUnexported = cmpopts.IgnoreUnexported(admiralV1Alpha1.GlobalTrafficPolicy{}.Status)

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
		GatewayAssetAliases:                 []string{"mock.gateway", "Org.platform.servicesgateway.servicesgateway"},
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.AdmiralCRDIdentityLabel = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	p.LabelSet.IdentityPartitionKey = "admiral.io/identityPartition"
	p.LabelSet.DeploymentAnnotation = "sidecar.istio.io/inject"
	p.LabelSet.AdmiralIgnoreLabel = "admiral.io/ignore"
	InitializeConfig(p)
}

func TestGetTrafficConfigTransactionID(t *testing.T) {
	tc := admiralV1Alpha1.TrafficConfig{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"transactionID": ""},
			Annotations: map[string]string{
				"transactionID": "123456",
			}},
	}
	tid := GetTrafficConfigTransactionID(&tc)
	assert.NotNil(t, tid)
}

func TestGetTrafficConfigRevision(t *testing.T) {
	tc := admiralV1Alpha1.TrafficConfig{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"revisionNumber": ""},
			Annotations: map[string]string{
				"revisionNumber": "123456",
			}},
	}
	tid := GetTrafficConfigRevision(&tc)
	assert.NotNil(t, tid)
}

func TestGetTrafficConfigIdentity(t *testing.T) {
	tc := admiralV1Alpha1.TrafficConfig{
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

	deployment := k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{identifier: identifierVal}}}}}
	deploymentWithAnnotation := k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal}}}}}

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
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   strings.ToLower("stage." + identifierVal + ".global"),
		}, {
			name:       "should return valid cname (from label) uses case sensitive DNS annotation -enabled",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"admiral.io/cname-case-sensitive": "true"}, Labels: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   "stage." + identifierVal + ".global",
		}, {
			name:       "should return valid cname (from label)  uses case sensitive DNS annotation -disabled",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"admiral.io/cname-case-sensitive": "false"}, Labels: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   strings.ToLower("stage." + identifierVal + ".global"),
		},
		{
			name:       "should return valid cname (from annotation)",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal}, Labels: map[string]string{"env": "stage"}}}}},
			expected:   strings.ToLower("stage." + identifierVal + ".global"),
		},
		{
			name:       "should return empty string",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"env": "stage"}}}}},
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
			node:     k8sCoreV1.Node{Spec: k8sCoreV1.NodeSpec{}, ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{NodeRegionLabel: nodeLocalityLabel}}},
			expected: nodeLocalityLabel,
		},
		{
			name:     "should return empty value when node annotation isn't present",
			node:     k8sCoreV1.Node{Spec: k8sCoreV1.NodeSpec{}, ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}}},
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
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   identifierVal,
			originalex: identifierVal,
		},
		{
			name:       "should return valid identifier from annotations",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   identifierVal,
			originalex: identifierVal,
		},
		{
			name:       "should return partitioned identifier",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal, "env": "stage", "admiral.io/identityPartition": "pid"}}}}},
			expected:   "pid." + identifierVal,
			originalex: identifierVal,
		},
		{
			name:       "should return empty identifier",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{}, Annotations: map[string]string{}}}}},
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
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{partitionIdentifier: identifierVal, "env": "stage"}}}}},
			expected:   identifierVal,
		},
		{
			name:       "should return valid identifier from annotations",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{partitionIdentifier: identifierVal, "env": "stage"}}}}},
			expected:   identifierVal,
		},
		{
			name:       "should return empty identifier",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{}, Annotations: map[string]string{}}}}},
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
			pod:      k8sCoreV1.Pod{Spec: k8sCoreV1.PodSpec{}, ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{identifier: identifierVal, "env": "stage"}}},
			expected: identifierVal,
		},
		{
			name:     "should return valid identifier from annotation",
			pod:      k8sCoreV1.Pod{Spec: k8sCoreV1.PodSpec{}, ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal, "env": "stage"}}},
			expected: identifierVal,
		},
		{
			name:     "should return empty identifier",
			pod:      k8sCoreV1.Pod{Spec: k8sCoreV1.PodSpec{}, ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}, Labels: map[string]string{}}},
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
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{}}}}},
			expected:   Default,
		},
		{
			name:       "should return valid env from label",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}, Labels: map[string]string{"env": "stage2"}}}}},
			expected:   "stage2",
		},
		{
			name:       "should return valid env from new annotation",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"admiral.io/env": "stage1"}, Labels: map[string]string{"env": "stage2"}}}}},
			expected:   "stage1",
		},
		{
			name:       "should return valid env from new label",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}, Labels: map[string]string{"admiral.io/env": "production", "env": "stage2"}}}}},
			expected:   "production",
		},
		{
			name:       "should return env from namespace suffix",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{}}}}, ObjectMeta: metav1.ObjectMeta{Namespace: "uswest2-prd"}},
			expected:   "prd",
		},
		{
			name:       "should return default when namespace doesn't have blah..region-env format",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{}}}}, ObjectMeta: metav1.ObjectMeta{Namespace: "sample"}},
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

	envNewAnnotationGtp := admiralV1Alpha1.GlobalTrafficPolicy{}
	envNewAnnotationGtp.CreationTimestamp = metav1.Now()
	envNewAnnotationGtp.Labels = map[string]string{"identity": "app1", "admiral.io/env": "stage1"}
	envNewAnnotationGtp.Annotations = map[string]string{"admiral.io/env": "production"}
	envNewAnnotationGtp.Namespace = "namespace"
	envNewAnnotationGtp.Name = "myGTP-new-annotation"

	envNewLabelGtp := admiralV1Alpha1.GlobalTrafficPolicy{}
	envNewLabelGtp.CreationTimestamp = metav1.Now()
	envNewLabelGtp.Labels = map[string]string{"identity": "app1", "admiral.io/env": "stage1", "env": "stage2"}
	envNewLabelGtp.Namespace = "namespace"
	envNewLabelGtp.Name = "myGTP-new-label"

	envLabelGtp := admiralV1Alpha1.GlobalTrafficPolicy{}
	envLabelGtp.CreationTimestamp = metav1.Now()
	envLabelGtp.Labels = map[string]string{"identity": "app1", "env": "stage2"}
	envLabelGtp.Namespace = "namespace"
	envLabelGtp.Name = "myGTP-label"

	noEnvGtp := admiralV1Alpha1.GlobalTrafficPolicy{}
	noEnvGtp.CreationTimestamp = metav1.Now()
	noEnvGtp.Labels = map[string]string{"identity": "app1"}
	noEnvGtp.Namespace = "namespace"
	noEnvGtp.Name = "myGTP-no-env"

	testCases := []struct {
		name        string
		gtp         *admiralV1Alpha1.GlobalTrafficPolicy
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

	envNewAnnotationRP := admiralV1Alpha1.RoutingPolicy{}
	envNewAnnotationRP.CreationTimestamp = metav1.Now()
	envNewAnnotationRP.Labels = map[string]string{"identity": "app1", "admiral.io/env": "stage1"}
	envNewAnnotationRP.Annotations = map[string]string{"identity": "app1", "admiral.io/env": "stage1"}
	envNewAnnotationRP.Namespace = "namespace"
	envNewAnnotationRP.Name = "myRP-new-annotation"

	envLabelRP := admiralV1Alpha1.RoutingPolicy{}
	envLabelRP.CreationTimestamp = metav1.Now()
	envLabelRP.Labels = map[string]string{"admiral.io/env": "stage1", "env": "stage2"}
	envLabelRP.Namespace = "namespace"
	envLabelRP.Name = "myRP-label"

	noEnvRP := admiralV1Alpha1.RoutingPolicy{}
	noEnvRP.CreationTimestamp = metav1.Now()
	noEnvRP.Namespace = "namespace"
	noEnvRP.Name = "myRP-no-env"

	testCases := []struct {
		name        string
		rp          *admiralV1Alpha1.RoutingPolicy
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

	gtpIdentityFromLabels := admiralV1Alpha1.GlobalTrafficPolicy{}
	gtpIdentityFromLabels.CreationTimestamp = metav1.Now()
	gtpIdentityFromLabels.Labels = map[string]string{"identity": "app1", "admiral.io/env": "stage1"}
	gtpIdentityFromLabels.Annotations = map[string]string{"admiral.io/env": "production"}
	gtpIdentityFromLabels.Namespace = "namespace"
	gtpIdentityFromLabels.Name = "myGTP"

	gtpIdenityFromSelector := admiralV1Alpha1.GlobalTrafficPolicy{}
	gtpIdenityFromSelector.CreationTimestamp = metav1.Now()
	gtpIdenityFromSelector.Labels = map[string]string{"admiral.io/env": "stage1", "env": "stage2"}
	gtpIdenityFromSelector.Spec.Selector = map[string]string{"identity": "app2", "admiral.io/env": "stage1", "env": "stage2"}
	gtpIdenityFromSelector.Namespace = "namespace"
	gtpIdenityFromSelector.Name = "myGTP"

	testCases := []struct {
		name             string
		gtp              *admiralV1Alpha1.GlobalTrafficPolicy
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
	matchingSelector := metav1.LabelSelector{}
	matchingSelector.MatchLabels = map[string]string{"app": "app1", "asset": "asset1"}
	matchingServiceSelector := map[string]string{"app": "app1", "asset": "asset1"}

	nonMatchingSelector := metav1.LabelSelector{}
	nonMatchingSelector.MatchLabels = map[string]string{"app": "app1", "asset": "asset1"}
	nonMatchingServiceSelector := map[string]string{"app": "app2", "asset": "asset1"}

	nilSelector := metav1.LabelSelector{}
	nonNilServiceSelector := map[string]string{"app": "app1", "asset": "asset1"}

	nonNilSelector := metav1.LabelSelector{}
	nonNilSelector.MatchLabels = map[string]string{"app": "app1", "asset": "asset1"}
	nilServiceSelector := map[string]string{}
	testCases := []struct {
		name            string
		selector        *metav1.LabelSelector
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
	rp := &admiralV1Alpha1.RoutingPolicy{
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
			rp := &admiralV1Alpha1.RoutingPolicy{
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
		od *admiralV1Alpha1.OutlierDetection
	}

	test1od := &admiralV1Alpha1.OutlierDetection{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       model.OutlierDetection{},
		Status:     admiralV1Alpha1.OutlierDetectionStatus{},
	}
	test1od.Labels = make(map[string]string)
	test1od.Labels["identity"] = "foo"

	test2od := &admiralV1Alpha1.OutlierDetection{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       model.OutlierDetection{},
		Status:     admiralV1Alpha1.OutlierDetectionStatus{},
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
		od *admiralV1Alpha1.OutlierDetection
	}

	test1od := &admiralV1Alpha1.OutlierDetection{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       model.OutlierDetection{},
		Status:     admiralV1Alpha1.OutlierDetectionStatus{},
	}
	test1od.Labels = make(map[string]string)

	test2od := &admiralV1Alpha1.OutlierDetection{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       model.OutlierDetection{},
		Status:     admiralV1Alpha1.OutlierDetectionStatus{},
	}
	test2od.Annotations = make(map[string]string)
	test2od.Annotations["admiral.io/env"] = "fooAnnotation"

	test3od := &admiralV1Alpha1.OutlierDetection{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       model.OutlierDetection{},
		Status:     admiralV1Alpha1.OutlierDetectionStatus{},
	}
	test3od.Labels = make(map[string]string)
	test3od.Labels["admiral.io/env"] = "fooLabel"

	test4od := &admiralV1Alpha1.OutlierDetection{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       model.OutlierDetection{},
		Status:     admiralV1Alpha1.OutlierDetectionStatus{},
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
	tc := admiralV1Alpha1.TrafficConfig{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"env": ""},
			Annotations: map[string]string{
				"asset": "123456",
			}},
	}
	tid := CheckIFEnvLabelIsPresent(&tc)
	assert.NotNil(t, tid)
}

func TestCheckIFEnvLabelIsPresentLabelsMissing(t *testing.T) {
	tc := admiralV1Alpha1.TrafficConfig{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"asset": "123456",
			}},
	}
	tid := CheckIFEnvLabelIsPresent(&tc)
	assert.NotNil(t, tid)
}

func TestCheckIFEnvLabelIsPresentSuccess(t *testing.T) {
	tc := admiralV1Alpha1.TrafficConfig{
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
		meta     metav1.Object
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

func TestGetMeshPorts(t *testing.T) {
	var (
		annotatedPort               = 8090
		annotatedSecondPort         = 8091
		defaultServicePort          = uint32(8080)
		ports                       = map[string]uint32{"http": uint32(annotatedPort)}
		portsDiffTargetPort         = map[string]uint32{"http": uint32(80)}
		grpcPorts                   = map[string]uint32{"grpc": uint32(annotatedPort)}
		grpcWebPorts                = map[string]uint32{"grpc-web": uint32(annotatedPort)}
		http2Ports                  = map[string]uint32{"http2": uint32(annotatedPort)}
		portsFromDefaultSvcPort     = map[string]uint32{"http": defaultServicePort}
		emptyPorts                  = map[string]uint32{}
		defaultK8sSvcPortNoName     = k8sCoreV1.ServicePort{Port: int32(defaultServicePort)}
		defaultK8sSvcPort           = k8sCoreV1.ServicePort{Name: "default", Port: int32(defaultServicePort)}
		meshK8sSvcPort              = k8sCoreV1.ServicePort{Name: "mesh", Port: int32(annotatedPort)}
		serviceMeshPorts            = []k8sCoreV1.ServicePort{defaultK8sSvcPort, meshK8sSvcPort}
		serviceMeshPortsOnlyDefault = []k8sCoreV1.ServicePort{defaultK8sSvcPortNoName}
		service                     = k8sCoreV1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
			Spec:       k8sCoreV1.ServiceSpec{Ports: serviceMeshPorts},
		}
		deployment = k8sAppsV1.Deployment{
			Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{SidecarEnabledPorts: strconv.Itoa(annotatedPort)}},
			}}}
		deploymentWithMultipleMeshPorts = k8sAppsV1.Deployment{
			Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{SidecarEnabledPorts: strconv.Itoa(annotatedPort) + "," + strconv.Itoa(annotatedSecondPort)}},
			}}}
	)

	testCases := []struct {
		name        string
		clusterName string
		service     k8sCoreV1.Service
		deployment  k8sAppsV1.Deployment
		expected    map[string]uint32
	}{
		{
			name:       "should return a port based on annotation",
			service:    service,
			deployment: deployment,
			expected:   ports,
		},
		{
			name: "should return a http port if no port name is specified",
			service: k8sCoreV1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sCoreV1.ServiceSpec{Ports: []k8sCoreV1.ServicePort{{Port: int32(80), TargetPort: intstr.FromInt(annotatedPort)}}},
			},
			deployment: deployment,
			expected:   portsDiffTargetPort,
		},
		{
			name: "should return a http port if the port name doesn't start with a protocol name",
			service: k8sCoreV1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sCoreV1.ServiceSpec{Ports: []k8sCoreV1.ServicePort{{Name: "hello-grpc", Port: int32(annotatedPort)}}},
			},
			deployment: deployment,
			expected:   ports,
		},
		{
			name: "should return a grpc port based on annotation",
			service: k8sCoreV1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sCoreV1.ServiceSpec{Ports: []k8sCoreV1.ServicePort{{Name: "grpc-service", Port: int32(annotatedPort)}}},
			},
			deployment: deployment,
			expected:   grpcPorts,
		},
		{
			name: "should return a grpc-web port based on annotation",
			service: k8sCoreV1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sCoreV1.ServiceSpec{Ports: []k8sCoreV1.ServicePort{{Name: "grpc-web", Port: int32(annotatedPort)}}},
			},
			deployment: deployment,
			expected:   grpcWebPorts,
		},
		{
			name: "should return a http2 port based on annotation",
			service: k8sCoreV1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sCoreV1.ServiceSpec{Ports: []k8sCoreV1.ServicePort{{Name: "http2", Port: int32(annotatedPort)}}},
			},
			deployment: deployment,
			expected:   http2Ports,
		},
		{
			name: "should return a default port",
			service: k8sCoreV1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sCoreV1.ServiceSpec{Ports: serviceMeshPortsOnlyDefault},
			},
			deployment: k8sAppsV1.Deployment{
				Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
				}}},
			expected: portsFromDefaultSvcPort,
		},
		{
			name: "should return empty ports",
			service: k8sCoreV1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec:       k8sCoreV1.ServiceSpec{Ports: nil},
			},
			deployment: k8sAppsV1.Deployment{
				Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
				}}},
			expected: emptyPorts,
		},
		{
			name: "should return a http port if the port name doesn't start with a protocol name",
			service: k8sCoreV1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "server", Labels: map[string]string{"asset": "Intuit.platform.mesh.server"}},
				Spec: k8sCoreV1.ServiceSpec{Ports: []k8sCoreV1.ServicePort{{Name: "http", Port: int32(annotatedPort)},
					{Name: "grpc", Port: int32(annotatedSecondPort)}}},
			},
			deployment: deploymentWithMultipleMeshPorts,
			expected:   ports,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			meshPorts := GetMeshPortsForDeployments(c.clusterName, &c.service, &c.deployment)
			if !reflect.DeepEqual(meshPorts, c.expected) {
				t.Errorf("Wanted meshPorts: %v, got: %v", c.expected, meshPorts)
			}
		})
	}
}

func TestGetGtpIdentityPartition(t *testing.T) {
	initConfig(true, true)
	partitionIdentifier := "admiral.io/identityPartition"
	identifierVal := "swX"

	testCases := []struct {
		name     string
		gtp      admiralV1Alpha1.GlobalTrafficPolicy
		expected string
	}{
		{
			name:     "should return valid identifier from label",
			gtp:      admiralV1Alpha1.GlobalTrafficPolicy{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{partitionIdentifier: identifierVal, "env": "stage"}}},
			expected: identifierVal,
		},
		{
			name:     "should return valid identifier from annotations",
			gtp:      admiralV1Alpha1.GlobalTrafficPolicy{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{partitionIdentifier: identifierVal, "env": "stage"}}},
			expected: identifierVal,
		},
		{
			name:     "should return empty identifier",
			gtp:      admiralV1Alpha1.GlobalTrafficPolicy{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}, Annotations: map[string]string{}}},
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

func TestShouldIgnore(t *testing.T) {
	initConfig(true, true)

	testCases := []struct {
		name        string
		annotations map[string]string
		labels      map[string]string
		expected    bool
	}{
		{
			name: "Given valid admiral ignore label " +
				"Should ignore the object ",
			annotations: map[string]string{"sidecar.istio.io/inject": "true"},
			labels:      map[string]string{"admiral.io/ignore": "true", "app": "app"},
			expected:    true,
		},
		{
			name: "Given istio injection is not enabled " +
				"Should ignore the object ",
			annotations: map[string]string{},
			labels:      map[string]string{"app": "app"},
			expected:    true,
		},
		{
			name: "Given valid admiral ignore annotation " +
				"Should ignore the object ",
			annotations: map[string]string{"admiral.io/ignore": "true", "sidecar.istio.io/inject": "true"},
			labels:      map[string]string{"app": "app"},
			expected:    true,
		},
		{
			name: "Given no admiral ignore set " +
				"Should not ignore the object ",
			annotations: map[string]string{"sidecar.istio.io/inject": "true"},
			labels:      map[string]string{"app": "app"},
			expected:    false,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			iVal := ShouldIgnore(c.annotations, c.labels)
			if !(iVal == c.expected) {
				t.Errorf("Wanted value: %v, got: %v", c.expected, iVal)
			}
		})
	}
}

func TestGetIdentityPartition(t *testing.T) {
	initConfig(true, true)

	testCases := []struct {
		name        string
		annotations map[string]string
		labels      map[string]string
		expected    string
	}{
		{
			name: "Given valid identity partition on annotations " +
				"Should return identity partition ",
			annotations: map[string]string{"admiral.io/identityPartition": "partition1"},
			labels:      map[string]string{"app": "app"},
			expected:    "partition1",
		},
		{
			name: "Given valid identity partition on labels " +
				"Should return identity partition ",
			annotations: map[string]string{},
			labels:      map[string]string{"app": "app", "admiral.io/identityPartition": "partition2"},
			expected:    "partition2",
		},
		{
			name: "Given no valid identity partition present on labels or annotations " +
				"Should return empty string ",
			annotations: map[string]string{},
			labels:      map[string]string{"app": "app"},
			expected:    "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			iVal := GetIdentityPartition(c.annotations, c.labels)
			if !(iVal == c.expected) {
				t.Errorf("Wanted value: %v, got: %v", c.expected, iVal)
			}
		})
	}
}

func TestGetGlobalIdentifier(t *testing.T) {
	initConfig(true, true)

	testCases := []struct {
		name        string
		annotations map[string]string
		labels      map[string]string
		expected    string
	}{
		{
			name: "Given valid identity partition on annotations and valid identity" +
				"Should return global identifier with identity partition ",
			annotations: map[string]string{"admiral.io/identityPartition": "partition1"},
			labels:      map[string]string{"app": "app", "identity": "identity1"},
			expected:    "partition1.identity1",
		},
		{
			name: "Given valid identity partition on labels and valid identity " +
				"Should return global identifier with identity partition ",
			annotations: map[string]string{},
			labels:      map[string]string{"app": "app", "admiral.io/identityPartition": "partition2", "identity": "identity2"},
			expected:    "partition2.identity2",
		},
		{
			name: "Given no valid identity partition present on labels or annotations and valid identity " +
				"Should return identity string ",
			annotations: map[string]string{},
			labels:      map[string]string{"app": "app", "identity": "identity3"},
			expected:    "identity3",
		},
		{
			name: "Given no valid identity partition and no valid identity " +
				"Should empty string ",
			annotations: map[string]string{},
			labels:      map[string]string{"app": "app"},
			expected:    "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			iVal := GetGlobalIdentifier(c.annotations, c.labels)
			if !(iVal == c.expected) {
				t.Errorf("Wanted value: %v, got: %v", c.expected, iVal)
			}
		})
	}
}

func TestGetOriginalIdentifier(t *testing.T) {
	initConfig(true, true)

	testCases := []struct {
		name        string
		annotations map[string]string
		labels      map[string]string
		expected    string
	}{
		{
			name: "Given valid identity on annotations " +
				"Should return identity ",
			annotations: map[string]string{"identity": "identity1"},
			labels:      map[string]string{"app": "app"},
			expected:    "identity1",
		},
		{
			name: "Given valid identity on labels " +
				"Should return identity ",
			annotations: map[string]string{},
			labels:      map[string]string{"app": "app", "identity": "identity2"},
			expected:    "identity2",
		},
		{
			name: "Given no valid identity on labels or annotations " +
				"Should return empty identity ",
			annotations: map[string]string{},
			labels:      map[string]string{"app": "app"},
			expected:    "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			iVal := GetOriginalIdentifier(c.annotations, c.labels)
			if !(iVal == c.expected) {
				t.Errorf("Wanted value: %v, got: %v", c.expected, iVal)
			}
		})
	}
}

func TestGenerateUniqueNameForVS(t *testing.T) {
	initConfig(true, true)

	testCases := []struct {
		name     string
		vsName   string
		nsName   string
		expected string
	}{
		{
			name: "Given valid namespace name and virtual service name" +
				"Should return valid vsname ",
			vsName:   "test-vs",
			nsName:   "test-ns",
			expected: "test-ns-test-vs",
		},
		{
			name: "Given valid namespace name and nil virtual service name" +
				"Should return valid vsname with only namespace name ",
			vsName:   "",
			nsName:   "test-ns",
			expected: "test-ns",
		},
		{
			name: "Given nil namespace name and valid virtual service name" +
				"Should return valid vsname with only vs name ",
			vsName:   "test-vs",
			nsName:   "",
			expected: "test-vs",
		},
		{
			name: "Given valid namespace name and virtual service name over 250 chars long" +
				"Should return valid truncated vsname",
			vsName:   "mbjwbsaabyxpitryeptqtgcwfkseodgqgvoccktivzfqlzdbxhctplqhqixuxpeqjcsrgzaxxbfphasmrkpkyeosxxljmsqalmfublwcecztwgdtkulcrfwiwqqza123",
			nsName:   "mbjwbsaabyxpitryeptqtgcwfkseodgqgvoccktivzfqlzdbxhctplqhqixuxpeqjcsrgzaxxbfphasmrkpkyeosxxljmsqalmfublwcecztwgdtkulcrfwiwqqza123",
			expected: "mbjwbsaabyxpitryeptqtgcwfkseodgqgvoccktivzfqlzdbxhctplqhqixuxpeqjcsrgzaxxbfphasmrkpkyeosxxljmsqalmfublwcecztwgdtkulcrfwiwqqza123-mbjwbsaabyxpitryeptqtgcwfkseodgqgvoccktivzfqlzdbxhctplqhqixuxpeqjcsrgzaxxbfphasmrkpkyeosxxljmsqalmfublwcecztwgdtkulcrfwiw",
		},
		{
			name: "Given nil namespace name and nil virtual service name" +
				"Should return nil vsname ",
			vsName:   "",
			nsName:   "",
			expected: "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			iVal := GenerateUniqueNameForVS(c.nsName, c.vsName)
			if !(iVal == c.expected) {
				t.Errorf("Wanted value: %s, got: %s", c.expected, iVal)
			}
		})
	}
}

func TestIsAGateway(t *testing.T) {
	initConfig(true, true)

	testCases := []struct {
		name     string
		asset    string
		expected bool
	}{
		{
			name: "Given valid GW asset name" +
				"Should return true",
			asset:    "sw1.org.platform.servicesgateway.servicesgateway",
			expected: true,
		},
		{
			name: "Given valid asset name that is not GW" +
				"Should return false",
			asset:    "foo.bar",
			expected: false,
		},
		{
			name: "Given nil asset name" +
				"Should return false",
			expected: false,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			iVal := IsAGateway(c.asset)
			if !(iVal == c.expected) {
				t.Errorf("Wanted value: %t, got: %t", c.expected, iVal)
			}
		})
	}
}

func TestGetPartitionAndOriginalIdentifierFromPartitionedIdentifier(t *testing.T) {
	initConfig(true, true)

	testCases := []struct {
		name                       string
		identifier                 string
		expectedPartition          string
		expectedOriginalIdentifier string
	}{
		{
			name: "Given valid partitioned identifier" +
				"Should return partition and original identifier",
			identifier:                 "sw1.org.platform.servicesgateway.servicesgateway",
			expectedPartition:          "sw1",
			expectedOriginalIdentifier: "Org.platform.servicesgateway.servicesgateway",
		},
		{
			name: "Given gateway asset alias" +
				"Should return partition and original identifier",
			identifier:                 "Org.platform.servicesgateway.servicesgateway",
			expectedPartition:          "",
			expectedOriginalIdentifier: "Org.platform.servicesgateway.servicesgateway",
		},
		{
			name: "Given non partitioned identifier" +
				"Should return empty partition and original identifier",
			identifier:                 "Org.foo.bar",
			expectedPartition:          "",
			expectedOriginalIdentifier: "Org.foo.bar",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			partition, originalIdentifier := GetPartitionAndOriginalIdentifierFromPartitionedIdentifier(c.identifier)
			if !(partition == c.expectedPartition && originalIdentifier == c.expectedOriginalIdentifier) {
				t.Errorf("Wanted partition: %s, original identifier: %s, got: %s, %s", c.expectedPartition, c.expectedOriginalIdentifier, partition, originalIdentifier)
			}
		})
	}
}

func TestGetGtpPreferenceRegion(t *testing.T) {
	testCases := []struct {
		name           string
		existingGtp    *admiralV1Alpha1.GlobalTrafficPolicy
		newGtp         *admiralV1Alpha1.GlobalTrafficPolicy
		expectedRegion string
	}{
		{
			name: "Returns current region for same GTP",
			existingGtp: &admiralV1Alpha1.GlobalTrafficPolicy{
				Spec: model.GlobalTrafficPolicy{
					Policy: []*model.TrafficPolicy{
						{
							LbType:    1,
							DnsPrefix: "west",
							Target: []*model.TrafficGroup{
								{
									Region: "us-west-2",
									Weight: 100,
								},
							},
						},
					},
				},
			},
			newGtp: &admiralV1Alpha1.GlobalTrafficPolicy{
				Spec: model.GlobalTrafficPolicy{
					Policy: []*model.TrafficPolicy{
						{
							LbType:    1,
							DnsPrefix: "west",
							Target: []*model.TrafficGroup{
								{
									Region: "us-west-2",
									Weight: 100,
								},
							},
						},
					},
				},
			},
			expectedRegion: "",
		},
		{
			name: "Returns error for different prefix count",
			existingGtp: &admiralV1Alpha1.GlobalTrafficPolicy{
				Spec: model.GlobalTrafficPolicy{
					Policy: []*model.TrafficPolicy{
						{
							LbType:    1,
							DnsPrefix: "west",
							Target: []*model.TrafficGroup{
								{
									Region: "us-west-2",
									Weight: 100,
								},
							},
						},
					},
				},
			},
			newGtp: &admiralV1Alpha1.GlobalTrafficPolicy{
				Spec: model.GlobalTrafficPolicy{
					Policy: []*model.TrafficPolicy{
						{
							LbType:    1,
							DnsPrefix: "west",
							Target: []*model.TrafficGroup{
								{
									Region: "us-east-2",
									Weight: 100,
								},
							},
						},
						{
							LbType:    1,
							DnsPrefix: "east",
							Target: []*model.TrafficGroup{
								{
									Region: "us-west-2",
									Weight: 100,
								},
							},
						},
					},
				},
			},
			expectedRegion: "us-east-2",
		},
		{
			name: "Returns no region for different weight",
			existingGtp: &admiralV1Alpha1.GlobalTrafficPolicy{
				Spec: model.GlobalTrafficPolicy{
					Policy: []*model.TrafficPolicy{
						{
							LbType:    1,
							DnsPrefix: "west",
							Target: []*model.TrafficGroup{
								{
									Region: "us-west-2",
									Weight: 100,
								},
							},
						},
					},
				},
			},
			newGtp: &admiralV1Alpha1.GlobalTrafficPolicy{
				Spec: model.GlobalTrafficPolicy{
					Policy: []*model.TrafficPolicy{
						{
							LbType:    1,
							DnsPrefix: "west",
							Target: []*model.TrafficGroup{
								{
									Region: "us-west-2",
									Weight: 80,
								},
							},
						},
					},
				},
			},
			expectedRegion: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			region := GetGtpPreferenceRegion(tc.existingGtp, tc.newGtp)

			if region != tc.expectedRegion {
				t.Errorf("got region %s; want %s", region, tc.expectedRegion)
			}
		})
	}
}

func TestMakeDnsPrefixRegionMapping(t *testing.T) {
	testCases := []struct {
		name     string
		gtp      *admiralV1Alpha1.GlobalTrafficPolicy
		expected map[string]string
	}{
		{
			name: "valid mapping with 100 weight",
			gtp: &admiralV1Alpha1.GlobalTrafficPolicy{
				Spec: model.GlobalTrafficPolicy{
					Policy: []*model.TrafficPolicy{
						{
							LbType:    model.TrafficPolicy_FAILOVER,
							DnsPrefix: "test-prefix",
							Target: []*model.TrafficGroup{
								{
									Region: "us-west-2",
									Weight: int32(100),
								},
							},
						},
					},
				},
			},
			expected: map[string]string{"test-prefix": "us-west-2"},
		},
		{
			name: "no mapping with weight less than 100",
			gtp: &admiralV1Alpha1.GlobalTrafficPolicy{
				Spec: model.GlobalTrafficPolicy{
					Policy: []*model.TrafficPolicy{
						{
							LbType:    model.TrafficPolicy_FAILOVER,
							DnsPrefix: "test-prefix",
							Target: []*model.TrafficGroup{
								{
									Region: "us-west-2",
									Weight: int32(50),
								},
								{
									Region: "us-east-2",
									Weight: int32(50),
								},
							},
						},
					},
				},
			},
			expected: map[string]string{},
		},
		{
			name: "empty policy",
			gtp: &admiralV1Alpha1.GlobalTrafficPolicy{
				Spec: model.GlobalTrafficPolicy{
					Policy: []*model.TrafficPolicy{},
				},
			},
			expected: map[string]string{},
		},
		{
			name: "non-failover lb type",
			gtp: &admiralV1Alpha1.GlobalTrafficPolicy{
				Spec: model.GlobalTrafficPolicy{
					Policy: []*model.TrafficPolicy{
						{
							LbType:    model.TrafficPolicy_TOPOLOGY, // not FAILOVER
							DnsPrefix: "test-prefix",
							Target: []*model.TrafficGroup{
								{
									Region: "us-east-2",
									Weight: int32(100),
								},
							},
						},
					},
				},
			},
			expected: map[string]string{},
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := makeDnsPrefixRegionMapping(c.gtp)
			if !reflect.DeepEqual(result, c.expected) {
				t.Errorf("Wanted DNS region mapping: %v, got: %v", c.expected, result)
			}
		})
	}
}

func TestSortGtpsByPriorityAndCreationTime(t *testing.T) {
	identity := "test-identity"
	env := "test"

	creationTime1, _ := time.Parse(time.RFC3339, "2024-12-05T06:54:17Z")
	creationTime2, _ := time.Parse(time.RFC3339, "2024-12-05T08:54:17Z")

	testCases := []struct {
		name        string
		gtpsToOrder []*admiralV1Alpha1.GlobalTrafficPolicy
		expected    []*admiralV1Alpha1.GlobalTrafficPolicy
	}{
		{
			name: "Give lastUpdatedAt is set for both GTPs" +
				"And priority is same" +
				"Then sort GTPs based on lastUpdatedAt and ignore creation time",
			gtpsToOrder: []*admiralV1Alpha1.GlobalTrafficPolicy{
				{
					ObjectMeta: v1.ObjectMeta{
						CreationTimestamp: v1.Time{creationTime1},
						Annotations: map[string]string{
							"lastUpdatedAt": "2024-12-05T06:54:17Z",
						},
						Labels: map[string]string{"priority": "10"},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						CreationTimestamp: v1.Time{creationTime2},
						Annotations: map[string]string{
							"lastUpdatedAt": "2024-12-05T08:54:17Z",
						},
						Labels: map[string]string{"priority": "10"},
					},
				},
			},
			expected: []*admiralV1Alpha1.GlobalTrafficPolicy{
				{
					ObjectMeta: v1.ObjectMeta{
						CreationTimestamp: v1.Time{creationTime2},
						Annotations: map[string]string{
							"lastUpdatedAt": "2024-12-05T08:54:17Z",
						},
						Labels: map[string]string{"priority": "10"},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						CreationTimestamp: v1.Time{creationTime1},
						Annotations: map[string]string{
							"lastUpdatedAt": "2024-12-05T06:54:17Z",
						},
						Labels: map[string]string{"priority": "10"},
					},
				},
			},
		},
		{
			name: "Give lastUpdatedAt is set for only one GTP" +
				"And priority is same" +
				"Then sort GTPs based on creation time and ignore lastUpdatedAt time",
			gtpsToOrder: []*admiralV1Alpha1.GlobalTrafficPolicy{
				{
					ObjectMeta: v1.ObjectMeta{
						CreationTimestamp: v1.Time{creationTime1},
						Annotations: map[string]string{
							"lastUpdatedAt": "2024-12-05T06:54:17Z",
						},
						Labels: map[string]string{"priority": "10"},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						CreationTimestamp: v1.Time{creationTime2},
						Labels:            map[string]string{"priority": "10"},
					},
				},
			},
			expected: []*admiralV1Alpha1.GlobalTrafficPolicy{
				{
					ObjectMeta: v1.ObjectMeta{
						CreationTimestamp: v1.Time{creationTime2},
						Labels:            map[string]string{"priority": "10"},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						CreationTimestamp: v1.Time{creationTime1},
						Annotations: map[string]string{
							"lastUpdatedAt": "2024-12-05T06:54:17Z",
						},
						Labels: map[string]string{"priority": "10"},
					},
				},
			},
		},
		{
			name: "Give creationTime is set for both GTPs" +
				"And priority is same and update time is not set" +
				"Then sort GTPs based on creationTime",
			gtpsToOrder: []*admiralV1Alpha1.GlobalTrafficPolicy{
				{
					ObjectMeta: v1.ObjectMeta{
						CreationTimestamp: v1.Time{creationTime1},
						Labels:            map[string]string{"priority": "10"},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						CreationTimestamp: v1.Time{creationTime2},
						Labels:            map[string]string{"priority": "10"},
					},
				},
			},
			expected: []*admiralV1Alpha1.GlobalTrafficPolicy{
				{
					ObjectMeta: v1.ObjectMeta{
						CreationTimestamp: v1.Time{creationTime2},
						Labels:            map[string]string{"priority": "10"},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						CreationTimestamp: v1.Time{creationTime1},
						Labels:            map[string]string{"priority": "10"},
					},
				},
			},
		},
		{
			name: "Given priority is set for both GTPs and different" +
				"Then sort GTPs based on priority and ignore other fields",
			gtpsToOrder: []*admiralV1Alpha1.GlobalTrafficPolicy{
				{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{
							"lastUpdatedAt": "2024-12-05T08:54:17Z",
						},
						CreationTimestamp: v1.Time{creationTime1},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{
							"lastUpdatedAt": "2024-12-05T06:54:17Z",
						},
						CreationTimestamp: v1.Time{creationTime2},
						Labels:            map[string]string{"priority": "1000000"},
					},
				},
			},
			expected: []*admiralV1Alpha1.GlobalTrafficPolicy{
				{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{
							"lastUpdatedAt": "2024-12-05T08:54:17Z",
						},
						CreationTimestamp: v1.Time{creationTime1},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{
							"lastUpdatedAt": "2024-12-05T06:54:17Z",
						},
						CreationTimestamp: v1.Time{creationTime2},
						Labels:            map[string]string{"priority": "1000000"},
					},
				},
			},
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			SortGtpsByPriorityAndCreationTime(c.gtpsToOrder, identity, env)
			for i := range c.gtpsToOrder {
				if !reflect.DeepEqual(c.gtpsToOrder[i], c.expected[i]) {
					t.Errorf("Wanted the GTP sorting at this index: %v to be: %v, got: %v", i, c.expected[i], c.gtpsToOrder[i])
				}
			}

		})
	}
}

func TestIsIstioIngressGatewayService(t *testing.T) {
	svc1 := k8sCoreV1.Service{
		TypeMeta:   v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{},
		Spec:       k8sCoreV1.ServiceSpec{},
		Status:     k8sCoreV1.ServiceStatus{},
	}

	svc2 := svc1
	svc2.Labels = map[string]string{"app": "istio-ingressgateway"}
	svc2.Namespace = NamespaceIstioSystem

	svc3 := svc2
	svc3.Namespace = NamespaceIstioSystem + "_TEST"

	svc4 := svc1
	svc4.Namespace = NamespaceIstioSystem

	type args struct {
		svc *k8sCoreV1.Service
		key string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"When Empty K8S Service is present then expect no exception & result is false", args{&svc1, "istio-ingressgateway"}, false},
		{"When correct K8S service is present then expect no exception & result is true", args{&svc2, "istio-ingressgateway"}, true},
		{"When K8S service containts wrong namespace then expect no exception & result is false", args{&svc3, "istio-ingressgateway"}, false},
		{"When K8S service don't have correct label then expect no exception & result is false", args{&svc4, "istio-ingressgateway"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, IsIstioIngressGatewayService(tt.args.svc, tt.args.key), "IsIstioIngressGatewayService(%v, %v)", tt.args.svc, tt.args.key)
		})
	}
}

func TestIsVSRoutingEnabledVirtualService(t *testing.T) {

	testCases := []struct {
		name           string
		vs             *v1alpha4.VirtualService
		expectedResult bool
	}{
		{
			name: "Given vs is nil" +
				"When IsVSRoutingEnabledVirtualService is called" +
				"Then func should return false ",
			vs:             nil,
			expectedResult: false,
		},
		{
			name: "Given vs annotations is nil" +
				"When IsVSRoutingEnabledVirtualService is called" +
				"Then func should return false ",
			vs:             &v1alpha4.VirtualService{},
			expectedResult: false,
		},
		{
			name: "Given vs which is not vs routing enabled" +
				"When IsVSRoutingEnabledVirtualService is called" +
				"Then func should return false ",
			vs: &v1alpha4.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"other-annotation": "true"},
				},
			},
			expectedResult: false,
		},
		{
			name: "Given vs which is vs routing enabled" +
				"When IsVSRoutingEnabledVirtualService is called" +
				"Then func should return true ",
			vs: &v1alpha4.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{VSRoutingLabel: "enabled"},
				},
			},
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := IsVSRoutingEnabledVirtualService(tc.vs)
			assert.Equal(t, tc.expectedResult, actual)
		})
	}

}

func TestIsVSRoutingInClusterVirtualService(t *testing.T) {

	testCases := []struct {
		name           string
		vs             *v1alpha4.VirtualService
		expectedResult bool
	}{
		{
			name: "Given vs is nil" +
				"When IsVSRoutingInClusterVirtualService is called" +
				"Then func should return false ",
			vs:             nil,
			expectedResult: false,
		},
		{
			name: "Given vs annotations is nil" +
				"When IsVSRoutingInClusterVirtualService is called" +
				"Then func should return false ",
			vs:             &v1alpha4.VirtualService{},
			expectedResult: false,
		},
		{
			name: "Given vs which is not vs routing enabled" +
				"When IsVSRoutingInClusterVirtualService is called" +
				"Then func should return false ",
			vs: &v1alpha4.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"other-annotation": "true"},
				},
			},
			expectedResult: false,
		},
		{
			name: "Given vs which is vs routing enabled" +
				"When IsVSRoutingInClusterVirtualService is called" +
				"Then func should return true ",
			vs: &v1alpha4.VirtualService{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{VSRoutingType: VSRoutingTypeInCluster},
				},
			},
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := IsVSRoutingInClusterVirtualService(tc.vs)
			assert.Equal(t, tc.expectedResult, actual)
		})
	}

}
