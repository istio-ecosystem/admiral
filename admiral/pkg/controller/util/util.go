package util

import (
	log "github.com/sirupsen/logrus"
	"reflect"
	"time"
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
	return true
}

func Contains(vs []string, t string) bool {
	for _, v := range vs {
		if v == t {
			return true
		}
	}
	return false
}

func LogElapsedTime(op, identity, env, clusterId string) func() {
	start := time.Now()
	return func() {
		LogElapsedTimeSince(op, identity, env, clusterId, start)
	}
}

func LogElapsedTimeSince(op, identity, env, clusterId string, start time.Time) {
	log.Infof("op=%s identity=%s env=%s cluster=%s time=%v\n", op, identity, env, clusterId, time.Since(start).Milliseconds())
}
