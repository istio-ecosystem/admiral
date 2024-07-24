package util

import (
	"bytes"
	"reflect"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestCopyMap(t *testing.T) {
	t.Parallel()
	var empty = map[string]string{}
	var m1 = map[string]string{"env": "stage", "version": "v1"}

	testCases := []struct {
		name     string
		input    map[string]string
		expected map[string]string
	}{
		{
			name:     "m1 is copied",
			input:    m1,
			expected: m1,
		},
		{
			name:     "empty is copied as is",
			input:    empty,
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
	var empty = map[string]string{}
	var m1 = map[string]string{"env": "stage", "version": "v1"}
	var m2 = map[string]string{"env": "stage", "version": "v1", "identity": "Admiral.platform.service.global"}

	testCases := []struct {
		name   string
		m1     map[string]string
		m2     map[string]string
		result bool
	}{
		{
			name:   "m1 is a subset of m2",
			m1:     m1,
			m2:     m2,
			result: true,
		},
		{
			name:   "m1 is a subset of m2",
			m1:     m1,
			m2:     m1,
			result: true,
		},
		{
			name:   "m1 is not a subset of m2",
			m1:     m2,
			m2:     m1,
			result: false,
		},
		{
			name:   "m1 is not a subset of m2",
			m1:     nil,
			m2:     m2,
			result: false,
		},
		{
			name:   "empty set is not a subset of m2",
			m1:     empty,
			m2:     m2,
			result: false,
		},
		{
			name:   "non-empty m1 is not a subset of non-empty m2 due to value mis-match",
			m1:     map[string]string{"env": "e2e", "version": "v1"},
			m2:     map[string]string{"env": "stage", "version": "v1"},
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

func TestContains(t *testing.T) {
	t.Parallel()
	var a1 = []string{"env", "stage", "version"}
	var a2 = []string{"hello"}

	testCases := []struct {
		name   string
		array  []string
		str    string
		result bool
	}{
		{
			name:   "a1 contains stage",
			array:  a1,
			str:    "stage",
			result: true,
		},
		{
			name:   "a2 doesn't contain stage",
			array:  a2,
			str:    "stage",
			result: false,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := Contains(c.array, c.str)
			if c.result != result {
				t.Errorf("Wanted result: %v, got: %v", c.result, result)
			}
		})
	}
}

func TestLogElapsedTime(t *testing.T) {
	logFunc := LogElapsedTime("test_op", "test_identity", "test_env", "test_clusterId")
	oldOut := log.StandardLogger().Out
	buf := bytes.Buffer{}
	log.SetOutput(&buf)
	logFunc()

	assert.Contains(t, buf.String(), "op=test_op identity=test_identity env=test_env cluster=test_clusterId txTime=")
	log.SetOutput(oldOut)
}
