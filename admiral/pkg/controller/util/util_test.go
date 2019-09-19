package util

import (
	"testing"
)

func TestSubset(t *testing.T) {
	t.Parallel()
	var empty = map[string]string {}
	var m1 = map[string]string {"env": "stage", "version": "v1"}
	var m2 = map[string]string {"env": "stage", "version": "v1", "identity": "Admiral.platform.service.global"}

	testCases := []struct {
		name string
		m1   map[string]string
		m2   map[string]string
		result  bool
	}{
		{
			name: "m1 is a subset of m2",
			m1: m1,
			m2: m2,
			result: true,
		},
		{
			name: "m1 is a subset of m2",
			m1: m1,
			m2: m1,
			result: true,
		},
		{
			name: "m1 is not a subset of m2",
			m1: m2,
			m2: m1,
			result: false,
		},
		{
			name: "m1 is not a subset of m2",
			m1: nil,
			m2: m2,
			result: false,
		},
		{
			name: "empty set is not a subset of m2",
			m1: empty,
			m2: m2,
			result: false,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := Subset(c.m1, c.m2)
			if c.result != result {
				t.Errorf("Wanted result: %v, got: %v", c.result, result)
			}
		})
	}

}
