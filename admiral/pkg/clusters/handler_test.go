package clusters

import (
	"testing"
)

func TestIgnoreIstioResource(t *testing.T) {

	//Struct of test case info. Name is required.
	testCases := []struct {
		name string
		exportTo []string
		expectedResult bool
	}{
		{
			name: "Should return false when exportTo is not present",
			exportTo: nil,
			expectedResult: false,
		},
		{
			name: "Should return false when its exported to *",
			exportTo: []string {"*"},
			expectedResult: false,
		},
		{
			name: "Should return true when its exported to .",
			exportTo: []string {"."},
			expectedResult: true,
		},
		{
			name: "Should return true when its exported to a handful of namespaces",
			exportTo: []string {"namespace1", "namespace2"},
			expectedResult: true,
		},
	}

	//Run the test for every provided case
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			result := IgnoreIstioResource(c.exportTo)
			if result == c.expectedResult {
				//perfect
			} else {
				t.Errorf("Failed. Got %v, expected %v",result, c.expectedResult)
			}
		})
	}
}
