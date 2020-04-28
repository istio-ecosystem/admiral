package common

import (
	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	v12 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	"reflect"
	"testing"
	"time"
)

func init() {
	p := AdmiralParams{
		KubeconfigPath: "testdata/fake.config",
		LabelSet: &LabelSet{},
		EnableSAN: true,
		SANPrefix: "prefix",
		HostnameSuffix: "mesh",
		SyncNamespace: "ns",
		CacheRefreshDuration: time.Minute,
		ClusterRegistriesNamespace: "default",
		DependenciesNamespace: "default",
		SecretResolver: "",
		WorkloadSidecarName: "default",
		WorkloadSidecarUpdate: "disabled",

	}

	p.LabelSet.WorkloadIdentityKey="identity"
	p.LabelSet.GlobalTrafficDeploymentLabel="identity"

	InitializeConfig(p)
}

func TestGetEnvForRollout(t *testing.T) {

	testCases := []struct {
		name       string
		rollout argo.Rollout
		expected   string
	}{
		{
			name:       "should return default env",
			rollout: argo.Rollout{Spec: argo.RolloutSpec{Template:corev1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}}}},
			expected:   Default,
		},
		{
			name:       "should return valid env from label",
			rollout: argo.Rollout{Spec: argo.RolloutSpec{Template: corev1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{}, Labels: map[string]string{"env": "stage"}}}}},
			expected:   "stage",
		},
		{
			name:       "should return valid env from annotation",
			rollout: argo.Rollout{Spec:  argo.RolloutSpec{Template: corev1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"env": "stage"}}}}},
			expected:   "stage",
		},
		{
			name:       "should return env from namespace suffix",
			rollout: argo.Rollout{Spec:  argo.RolloutSpec{Template: corev1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}}}, ObjectMeta: v1.ObjectMeta{Namespace: "uswest2-prd"}},
			expected:   "prd",
		},
		{
			name:       "should return default when namespace doesn't have blah..region-env format",
			rollout: argo.Rollout{Spec:  argo.RolloutSpec{Template: corev1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}}}, ObjectMeta: v1.ObjectMeta{Namespace: "sample"}},
			expected:   Default,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			env := GetEnvForRollout(&c.rollout)
			if !(env == c.expected) {
				t.Errorf("Wanted Cname: %s, got: %s", c.expected, env)
			}
		})
	}
}


func TestMatchGTPsToRollout(t *testing.T) {
	rollout := argo.Rollout{}
	rollout.Namespace = "namespace"
	rollout.Name = "fake-app-rollout-qal"
	rollout.CreationTimestamp = v1.Now()
	rollout.Spec = argo.RolloutSpec{
		Template: corev1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env":"qal"},
			},
		},
	}
	rollout.Labels = map[string]string{"identity": "app1"}

	otherEnvRollout := argo.Rollout{}
	otherEnvRollout.Namespace = "namespace"
	otherEnvRollout.Name = "fake-app-rollout-qal"
	otherEnvRollout.CreationTimestamp = v1.Now()
	otherEnvRollout.Spec = argo.RolloutSpec{
		Template: corev1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env":"random"},
			},
		},
	}
	otherEnvRollout.Labels = map[string]string{"identity": "app1"}

	noEnvRollout := argo.Rollout{}
	noEnvRollout.Namespace = "namespace"
	noEnvRollout.Name = "fake-app-rollout-qal"
	noEnvRollout.CreationTimestamp = v1.Now()
	noEnvRollout.Spec = argo.RolloutSpec{
		Template: corev1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1"},
			},
		},
	}
	noEnvRollout.Labels = map[string]string{"identity": "app1"}

	e2eGtp := v12.GlobalTrafficPolicy{}
	e2eGtp.CreationTimestamp = v1.Now()
	e2eGtp.Labels = map[string]string{"identity": "app1", "env":"e2e"}
	e2eGtp.Namespace = "namespace"
	e2eGtp.Name = "myGTP-e2e"

	prfGtp := v12.GlobalTrafficPolicy{}
	prfGtp.CreationTimestamp = v1.Now()
	prfGtp.Labels = map[string]string{"identity": "app1", "env":"prf"}
	prfGtp.Namespace = "namespace"
	prfGtp.Name = "myGTP-prf"

	qalGtp := v12.GlobalTrafficPolicy{}
	qalGtp.CreationTimestamp = v1.Now()
	qalGtp.Labels = map[string]string{"identity": "app1", "env":"qal"}
	qalGtp.Namespace = "namespace"
	qalGtp.Name = "myGTP"

	qalGtpOld := v12.GlobalTrafficPolicy{}
	qalGtpOld.CreationTimestamp = v1.Date(2020, 1, 1, 1, 1, 1, 1, time.UTC)
	qalGtpOld.Labels = map[string]string{"identity": "app1", "env":"qal"}
	qalGtpOld.Namespace = "namespace"
	qalGtpOld.Name = "myGTP"

	noEnvGTP := v12.GlobalTrafficPolicy{}
	noEnvGTP.CreationTimestamp = v1.Now()
	noEnvGTP.Labels = map[string]string{"identity": "app1"}
	noEnvGTP.Namespace = "namespace"
	noEnvGTP.Name = "myGTP"

	noEnvGTPOld := v12.GlobalTrafficPolicy{}
	noEnvGTPOld.CreationTimestamp = v1.Date(2020, 1, 1, 1, 1, 1, 1, time.UTC)
	noEnvGTPOld.Labels = map[string]string{"identity": "app1"}
	noEnvGTPOld.Namespace = "namespace"
	noEnvGTPOld.Name = "myGTP"


	testCases := []struct {
		name string
		gtp *[]v12.GlobalTrafficPolicy
		rollout *argo.Rollout
		expectedGTP *v12.GlobalTrafficPolicy
	}{
		{
			name: "Should return no rollout when none have a matching env",
			gtp: &[]v12.GlobalTrafficPolicy{e2eGtp, prfGtp, qalGtp, qalGtpOld},
			rollout: &otherEnvRollout,
			expectedGTP: nil,

		},
		{
			name: "Should return no rollout when the GTP doesn't have an environment",
			gtp: &[]v12.GlobalTrafficPolicy{noEnvGTP, noEnvGTPOld},
			rollout: &otherEnvRollout,
			expectedGTP: nil,

		},
		{
			name: "Should return no rollout when no rollouts have an environment",
			gtp: &[]v12.GlobalTrafficPolicy{e2eGtp, prfGtp},
			rollout: &noEnvRollout,
			expectedGTP: nil,

		},
		{
			name: "Should match a GTP and rollout when both have no env label",
			gtp: &[]v12.GlobalTrafficPolicy{e2eGtp, prfGtp, qalGtp, qalGtpOld, noEnvGTP, noEnvGTPOld},
			rollout: &noEnvRollout,
			expectedGTP: &noEnvGTPOld,

		},
		{
			name: "Should return Match when there's one match",
			gtp: &[]v12.GlobalTrafficPolicy{qalGtp},
			rollout: &rollout,
			expectedGTP: &qalGtp,

		},
		{
			name: "Should return Match when there's one match from a bigger list",
			gtp: &[]v12.GlobalTrafficPolicy{e2eGtp, prfGtp, qalGtp},
			rollout: &rollout,
			expectedGTP: &qalGtp,

		},
		{
			name: "Should handle multiple matches properly",
			gtp: &[]v12.GlobalTrafficPolicy{e2eGtp, prfGtp, qalGtp, qalGtpOld},
			rollout: &rollout,
			expectedGTP: &qalGtpOld,

		},
		{
			name: "Should return nil when there's no match",
			gtp: &[]v12.GlobalTrafficPolicy{},
			rollout: &rollout,
			expectedGTP: nil,

		},
		{
			name: "Should return nil the rollout is invalid",
			gtp: &[]v12.GlobalTrafficPolicy{},
			rollout: &argo.Rollout{},
			expectedGTP: nil,

		},
	}


	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			returned := MatchGTPsToRollout(*c.gtp, c.rollout)
			if !cmp.Equal(returned, c.expectedGTP, ignoreUnexported) {
				t.Fatalf("Rollout mismatch. Diff: %v", cmp.Diff(returned, c.expectedGTP, ignoreUnexported))
			}
		})
	}
}

func TestGetRolloutGlobalIdentifier(t *testing.T) {

	identifier := "identity"
	identifierVal := "company.platform.server"

	testCases := []struct {
		name       string
		rollout argo.Rollout
		expected   string
	}{
		{
			name:       "should return valid identifier from label",
			rollout: argo.Rollout{Spec: argo.RolloutSpec{Template: corev1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   identifierVal,
		},
		{
			name:       "should return valid identifier from annotations",
			rollout: argo.Rollout{Spec: argo.RolloutSpec{Template: corev1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   identifierVal,
		},
		{
			name:       "should return empty identifier",
			rollout: argo.Rollout{Spec: argo.RolloutSpec{Template: corev1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}, Annotations: map[string]string{}}}}},
			expected:   "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			iVal := GetRolloutGlobalIdentifier(&c.rollout)
			if !(iVal == c.expected) {
				t.Errorf("Wanted identity value: %s, got: %s", c.expected, iVal)
			}
		})
	}
}

func TestMatchRolloutsToGTP(t *testing.T) {
	rollout := argo.Rollout{}
	rollout.Namespace = "namespace"
	rollout.Name = "fake-app-rollout-qal"
	rollout.CreationTimestamp = v1.Now()
	rollout.Spec = argo.RolloutSpec{
		Template: corev1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env":"qal"},
			},
		},
	}
	rollout.Labels = map[string]string{"identity": "app1"}

	rollout2 := argo.Rollout{}
	rollout2.Namespace = "namespace"
	rollout2.Name = "fake-app-rollout-e2e"
	rollout2.CreationTimestamp = v1.Now()
	rollout2.Spec = argo.RolloutSpec{
		Template: corev1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env":"e2e"},
			},
		},
	}
	rollout2.Labels = map[string]string{"identity": "app1"}

	rollout3 := argo.Rollout{}
	rollout3.Namespace = "namespace"
	rollout3.Name = "fake-app-rollout-prf-1"
	rollout3.CreationTimestamp = v1.Now()
	rollout3.Spec = argo.RolloutSpec{
		Template: corev1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env":"prf"},
			},
		},
	}
	rollout3.Labels = map[string]string{"identity": "app1"}

	rollout4 := argo.Rollout{}
	rollout4.Namespace = "namespace"
	rollout4.Name = "fake-app-rollout-prf-2"
	rollout4.CreationTimestamp = v1.Date(2020, 1, 1, 1, 1, 1, 1, time.UTC)
	rollout4.Spec = argo.RolloutSpec{
		Template: corev1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{"identity": "app1", "env":"prf"},
			},
		},
	}
	rollout4.Labels = map[string]string{"identity": "app1"}

	e2eGtp := v12.GlobalTrafficPolicy{}
	e2eGtp.Labels = map[string]string{"identity": "app1", "env":"e2e"}
	e2eGtp.Namespace = "namespace"
	e2eGtp.Name = "myGTP"

	prfGtp := v12.GlobalTrafficPolicy{}
	prfGtp.Labels = map[string]string{"identity": "app1", "env":"prf"}
	prfGtp.Namespace = "namespace"
	prfGtp.Name = "myGTP"

	//Struct of test case info. Name is required.
	testCases := []struct {
		name string
		gtp *v12.GlobalTrafficPolicy
		rollouts *[]argo.Rollout
		expectedRollouts []argo.Rollout
	}{
		{
			name: "Should return nil when none have a matching environment",
			gtp: &e2eGtp,
			rollouts: &[]argo.Rollout{rollout, rollout3, rollout4},
			expectedRollouts: nil,

		},
		{
			name: "Should return Match when there's one match",
			gtp: &e2eGtp,
			rollouts: &[]argo.Rollout{rollout2},
			expectedRollouts: []argo.Rollout{rollout2},

		},
		{
			name: "Should return Match when there's one match from a bigger list",
			gtp: &e2eGtp,
			rollouts: &[]argo.Rollout{rollout, rollout2, rollout3, rollout4},
			expectedRollouts: []argo.Rollout{rollout2},

		},
		{
			name: "Should return nil when there's no match",
			gtp: &e2eGtp,
			rollouts: &[]argo.Rollout{},
			expectedRollouts: nil,

		},
		{
			name: "Should return nil when the GTP is invalid",
			gtp: &v12.GlobalTrafficPolicy{},
			rollouts: &[]argo.Rollout{rollout},
			expectedRollouts: nil,

		},
		{
			name: "Returns multiple matches",
			gtp: &prfGtp,
			rollouts: &[]argo.Rollout{rollout, rollout2, rollout3, rollout4},
			expectedRollouts: []argo.Rollout{rollout3, rollout4},

		},

	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			returned := MatchRolloutsToGTP(c.gtp, *c.rollouts)
			if !cmp.Equal(returned, c.expectedRollouts) {
				t.Fatalf("Rollout mismatch. Diff: %v", cmp.Diff(returned, c.expectedRollouts))
			}
		})
	}
}

func TestGetSANForRollout(t *testing.T) {
	t.Parallel()

	identifier := "identity"
	identifierVal := "company.platform.server"
	domain := "preprd"

	rollout := argo.Rollout{Spec: argo.RolloutSpec{Template: corev1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{identifier: identifierVal}}}}}
	rolloutWithAnnotation := argo.Rollout{Spec:  argo.RolloutSpec{Template: corev1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal}}}}}

	rolloutWithNoIdentifier := argo.Rollout{}

	testCases := []struct {
		name       string
		rollout argo.Rollout
		domain     string
		wantSAN    string
	}{
		{
			name:       "should return valid SAN (from label)",
			rollout: rollout,
			domain:     domain,
			wantSAN:    "spiffe://" + domain + "/" + identifierVal,
		},
		{
			name:       "should return valid SAN (from annotation)",
			rollout: rolloutWithAnnotation,
			domain:     domain,
			wantSAN:    "spiffe://" + domain + "/" + identifierVal,
		},
		{
			name:       "should return valid SAN with no domain prefix",
			rollout: rollout,
			domain:     "",
			wantSAN:    "spiffe://" + identifierVal,
		},
		{
			name:       "should return empty SAN",
			rollout: rolloutWithNoIdentifier,
			domain:     domain,
			wantSAN:    "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			san := GetSANForRollout(c.domain, &c.rollout, identifier)
			if !reflect.DeepEqual(san, c.wantSAN) {
				t.Errorf("Wanted SAN: %s, got: %s", c.wantSAN, san)
			}
		})
	}

}

func TestGetCnameForRollout(t *testing.T) {

	nameSuffix := "global"
	identifier := "identity"
	identifierVal := "company.platform.server"

	testCases := []struct {
		name       string
		rollout argo.Rollout
		expected   string
	}{
		{
			name:       "should return valid cname (from label)",
			rollout: argo.Rollout{Spec: argo.RolloutSpec{Template: corev1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected:   "stage." + identifierVal + ".global",
		},
		{
			name:       "should return valid cname (from annotation)",
			rollout: argo.Rollout{Spec: argo.RolloutSpec{Template: corev1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{identifier: identifierVal}, Labels: map[string]string{"env": "stage"}}}}},
			expected:   "stage." + identifierVal + ".global",
		},
		{
			name:       "should return empty string",
			rollout: argo.Rollout{Spec: argo.RolloutSpec{Template: corev1.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{"env": "stage"}}}}},
			expected:   "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			cname := GetCnameForRollout(&c.rollout, identifier, nameSuffix)
			if !(cname == c.expected) {
				t.Errorf("Wanted Cname: %s, got: %s", c.expected, cname)
			}
		})
	}
}

