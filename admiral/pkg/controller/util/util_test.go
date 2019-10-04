package util

import (
	"reflect"
	"testing"
)


func TestCopyMap(t *testing.T) {
	t.Parallel()
	var empty = map[string]string {}
	var m1 = map[string]string {"env": "stage", "version": "v1"}

	testCases := []struct {
		name string
		input   map[string]string
		expected   map[string]string
	}{
		{
			name: "m1 is copied",
			input: m1,
			expected: m1,
		},
		{
			name: "empty is copied as is",
			input: empty,
			expected: empty,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			var eMap = make(map[string]string)
			MapCopy(eMap, c.input)
			if !reflect.DeepEqual(c.expected, eMap) {
				t.Errorf("Wanted result: %v, got: %v", c.expected, eMap)
			}
		})
	}

}

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
