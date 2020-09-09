package common

import (
	"github.com/google/go-cmp/cmp"
	"strings"
	"testing"
)

func TestMapOfMaps(t *testing.T) {
	t.Parallel()
	mapOfMaps := NewMapOfMaps()
	mapOfMaps.Put("pkey1", "dev.a.global1", "127.0.10.1")
	mapOfMaps.Put("pkey1", "dev.a.global2", "127.0.10.2")
	mapOfMaps.Put("pkey2", "qa.a.global", "127.0.10.1")
	mapOfMaps.Put("pkey3", "stage.a.global", "127.0.10.1")

	map1 := mapOfMaps.Get("pkey1")
	if map1 == nil || map1.Get("dev.a.global1") != "127.0.10.1" {
		t.Fail()
	}

	map1.Delete("dev.a.global2")

	map12 := mapOfMaps.Get("pkey1")
	if map12.Get("dev.a.global2") != "" {
		t.Fail()
	}

	mapOfMaps.Put("pkey4", "prod.a.global", "127.0.10.1")

	map2 := mapOfMaps.Get("pkey4")
	if map2 == nil || map2.Get("prod.a.global") != "127.0.10.1" {
		t.Fail()
	}

	mapOfMaps.Put("pkey4", "prod.a.global", "127.0.10.1")

	mapOfMaps.Delete("pkey2")
	map3 := mapOfMaps.Get("pkey2")
	if map3 != nil {
		t.Fail()
	}
}

func TestEgressMap(t *testing.T) {
	egressMap := NewSidecarEgressMap()
	payments, orders := "payments", "orders"
	paymentsEnv, ordersEnv := "prod", "staging"
	paymentsNs, ordersNs := payments+"-"+paymentsEnv, orders+"-"+ordersEnv
	paymentsFqdn, ordersFqdn := payments+"."+paymentsNs+"."+"svc.cluster.local", orders+"."+ordersNs+"."+"svc.cluster.local"
	paymentsCname, ordersCname := paymentsEnv+"."+payments+".global", ordersEnv+"."+orders+".global"
	paymentsSidecar, ordersSidecar := SidecarEgress{FQDN: paymentsFqdn, Namespace: paymentsNs, CNAMEs: map[string]string{paymentsCname: paymentsCname}}, SidecarEgress{FQDN: ordersFqdn, Namespace: ordersNs, CNAMEs: map[string]string{ordersCname: ordersCname}}
	egressMap.Put(payments, paymentsNs, paymentsFqdn, map[string]string{paymentsCname: paymentsCname})
	egressMap.Put(orders, ordersNs, ordersFqdn, map[string]string{ordersCname: ordersCname})

	ordersEgress := egressMap.Get("orders")

	if !cmp.Equal(ordersEgress[ordersNs], ordersSidecar) {
		t.Errorf("Orders egress object should match expected %v, got %v", ordersSidecar, ordersEgress[ordersNs])
		t.FailNow()
	}

	egressMap.Delete(orders)
	ordersEgress = egressMap.Get("orders")

	if ordersEgress != nil {
		t.Errorf("Delete object should delete the object %v", ordersEgress)
		t.FailNow()
	}

	egressMapForIter := egressMap.Map()

	if len(egressMapForIter) != 1 {
		t.Errorf("Egressmap should contains only one object %v", paymentsSidecar)
		t.FailNow()
	}
}

func TestAdmiralParams(t *testing.T) {
	admiralParams := AdmiralParams{SANPrefix: "custom.san.prefix"}
	admiralParamsStr := admiralParams.String()
	expectedContainsStr := "SANPrefix=custom.san.prefix"
	if !strings.Contains(admiralParamsStr, expectedContainsStr) {
		t.Errorf("AdmiralParams String doesn't have the expected Stringified value expected to contain %v", expectedContainsStr)
	}
}
