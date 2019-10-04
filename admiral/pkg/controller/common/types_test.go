package common

import (
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