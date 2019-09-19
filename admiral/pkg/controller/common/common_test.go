package common

import (
	k8sAppsV1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"strconv"
	"testing"
)

func TestGetLocalAddressForSe(t *testing.T) {
	t.Parallel()
	cacheWithEntry := NewMap()
	cacheWithNoEntry := NewMap()
	cacheWith255Entries := NewMap()
	cacheWithEntry.Put("dev.a.global", "127.0.10.1")

	for i := 1; i <= 255; i++ {
		cacheWith255Entries.Put(strconv.Itoa(i) + ".global", "127.0.10." + strconv.Itoa(i))
	}

	testCases := []struct {
		name   string
		seName   string
		seAddressCache  *Map
		wantAddess string
	}{
		{
			name: "should return new available address",
			seName: "dev.a.global",
			seAddressCache: cacheWithNoEntry,
			wantAddess: "127.0.10.1",
		},
		{
			name: "should return address from map",
			seName: "dev.a.global",
			seAddressCache: cacheWithEntry,
			wantAddess: "127.0.10.1",
		},
		{
			name: "should return new available address",
			seName: "dev.b.global",
			seAddressCache: cacheWithEntry,
			wantAddess: "127.0.10.2",
		},
		{
			name: "should return new available address in higher subnet",
			seName: "dev.a.global",
			seAddressCache: cacheWith255Entries,
			wantAddess: "127.0.11.1",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			seAddress := GetLocalAddressForSe(c.seName, c.seAddressCache)
			if c.wantAddess != "" {
				if !reflect.DeepEqual(seAddress, c.wantAddess) {
					t.Errorf("Wanted se address: %s, got: %s", c.wantAddess, seAddress)
				}
			} else {
				if seAddress != "" {
					t.Errorf("Unexpectedly found address: %s", seAddress)
				}
			}
		})
	}

}

func TestGetSAN(t *testing.T) {
	t.Parallel()

	identifier := "identity"
	identifierVal := "company.platform.server"
	domain := "preprd"

	deployment := k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template:v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels:map[string]string{identifier: identifierVal}}}}}

	deploymentWithNoIdentifier := k8sAppsV1.Deployment{}

	testCases := []struct {
		name   string
		deployment   k8sAppsV1.Deployment
		domain  string
		wantSAN string
	}{
		{
			name: "should return valid SAN",
			deployment: deployment,
			domain: domain,
			wantSAN: "spiffe://" + domain + "/" + identifierVal,
		},
		{
			name: "should return empty SAN",
			deployment: deploymentWithNoIdentifier,
			domain: domain,
			wantSAN: "",
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

	identifier := "identity"
	identifierVal := "company.platform.server"

	testCases := []struct {
		name   string
		deployment   k8sAppsV1.Deployment
		expected string
	}{
		{
			name:    "should return valid cname (from label)",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template:v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels:map[string]string{identifier: identifierVal, "env": "stage"}}}}},
			expected: "stage." + identifierVal + ".global",
		},
		{
			name:    "should return valid cname (from annotation)",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template:v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Annotations:map[string]string{identifier: identifierVal}, Labels:map[string]string{"env": "stage"}}}}},
			expected: "stage." + identifierVal + ".global",
		},
		{
			name:    "should return empty string",
			deployment: k8sAppsV1.Deployment{Spec: k8sAppsV1.DeploymentSpec{Template:v12.PodTemplateSpec{ObjectMeta: v1.ObjectMeta{Labels:map[string]string{"env": "stage"}}}}},
			expected: "",
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			cname := GetCname(&c.deployment, identifier)
			if !(cname == c.expected) {
				t.Errorf("Wanted Cname: %s, got: %s", c.expected, cname)
			}
		})
	}
}