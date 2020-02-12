package common

import (
	k8sAppsV1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"testing"
)

func TestGetSAN(t *testing.T) {
	t.Parallel()

	identifier := "identity"
	identifierVal := "company.platform.server"
	domain := "preprd"

	deployment := k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{identifier: identifierVal}}}}}
	deploymentWithAnnotation := k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal}}}}}

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
	identifierVal := "company.platform.server"

	testCases := []struct {
		name       string
		deployment k8sAppsV1.Deployment
		expected   string
	}{
		{
			name:       "should return valid cname (from label)",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   "stage." + identifierVal + ".global",
		},
		{
			name:       "should return valid cname (from annotation)",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal}, Labels: map[string]string{"env": "stage"}}}}},
			expected:   "stage." + identifierVal + ".global",
		},
		{
			name:       "should return empty string",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{"env": "stage"}}}}},
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
		node     v12.Node
		expected string
	}{
		{
			name:     "should return valid node region",
			node:     v12.Node{Spec: v12.NodeSpec{}, ObjectMeta: v1.ObjectMeta{Labels: map[string]string{NodeRegionLabel: nodeLocalityLabel}}},
			expected: nodeLocalityLabel,
		},
		{
			name:     "should return empty value when node annotation isn't present",
			node:     v12.Node{Spec: v12.NodeSpec{}, ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{}}},
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
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   identifierVal,
		},
		{
			name:       "should return valid identifier from annotations",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   identifierVal,
		},
		{
			name:       "should return empty identifier",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}, Annotations: map[string]string{}}}}},
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
		pod      v12.Pod
		expected string
	}{
		{
			name:     "should return valid identifier from label",
			pod:      v12.Pod{Spec: v12.PodSpec{}, ObjectMeta: v1.ObjectMeta{Labels: map[string]string{identifier: identifierVal, "env": "stage"}}},
			expected: identifierVal,
		},
		{
			name:     "should return valid identifier from annotation",
			pod:      v12.Pod{Spec: v12.PodSpec{}, ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal, "env": "stage"}}},
			expected: identifierVal,
		},
		{
			name:     "should return empty identifier",
			pod:      v12.Pod{Spec: v12.PodSpec{}, ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{}, Labels: map[string]string{}}},
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
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}}}},
			expected:   Default,
		},
		{
			name:       "should return valid env from label",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{}, Labels: map[string]string{"env": "stage"}}}}},
			expected:   "stage",
		},
		{
			name:       "should return valid env from annotation",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"env": "stage"}}}}},
			expected:   "stage",
		},
		{
			name:       "should return env from namespace suffix",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}}}, ObjectMeta: v1.ObjectMeta{Namespace: "uswest2-prd"}},
			expected:   "prd",
		},
		{
			name:       "should return default when namespace doesn't have blah..region-env format",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template: v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}}}, ObjectMeta: v1.ObjectMeta{Namespace: "sample"}},
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
