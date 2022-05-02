package common

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v12 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sCoreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"strings"
	"testing"
	"time"
)

var ignoreUnexported = cmpopts.IgnoreUnexported(v12.GlobalTrafficPolicy{}.Status)

func init() {
	p := AdmiralParams{
		KubeconfigPath:             "testdata/fake.config",
		LabelSet:                   &LabelSet{},
		EnableSAN:                  true,
		SANPrefix:                  "prefix",
		HostnameSuffix:             "mesh",
		SyncNamespace:              "ns",
		CacheRefreshDuration:       time.Minute,
		ClusterRegistriesNamespace: "default",
		DependenciesNamespace:      "default",
		SecretResolver:             "",
		WorkloadSidecarName:        "default",
		WorkloadSidecarUpdate:      "disabled",
		MetricsEnabled:             true,
	}

	p.LabelSet.WorkloadIdentityKey = "identity"
	p.LabelSet.GlobalTrafficDeploymentLabel = "identity"
	p.LabelSet.EnvKey = "admiral.io/env"
	InitializeConfig(p)
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

	identifier := "identity"
	identifierVal := "company.platform.server"

	testCases := []struct {
		name       string
		deployment k8sAppsV1.Deployment
		expected   string
	}{
		{
			name:       "should return valid identifier from label",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   identifierVal,
		},
		{
			name:       "should return valid identifier from annotations",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: k8sCoreV1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
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
			iVal := GetDeploymentGlobalIdentifier(&c.deployment)
			if !(iVal == c.expected) {
				t.Errorf("Wanted identity value: %s, got: %s", c.expected, iVal)
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