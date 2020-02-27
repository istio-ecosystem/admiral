package util

import (
	"reflect"
)

func MapCopy(dst, src interface{}) {
	dv, sv := reflect.ValueOf(dst), reflect.ValueOf(src)

	for _, k := range sv.MapKeys() {
		dv.SetMapIndex(k, sv.MapIndex(k))
	}
}

// Subset returns whether m1 is a subset of m2
func Subset(m1 map[string]string, m2 map[string]string) bool {
	//empty set is not a subset of any set
	if m1 == nil || m2 == nil || len(m1) == 0 || len(m2) < len(m1) {
		return false
	}
	for k, v := range m1 {
		if val, ok := m2[k]; ok {
			if !reflect.DeepEqual(val, v) {
				return false
			}
		}
	}
	return true;
}

func Contains(vs []string, t string) bool {
	for _, v := range vs {
		if v == t {
			return true
		}
	}
	return false
}