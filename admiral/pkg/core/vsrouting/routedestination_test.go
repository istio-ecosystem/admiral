package vsrouting

import (
	"testing"

	"github.com/stretchr/testify/require"
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
)

func TestDeepCopyInto(t *testing.T) {

	testCases := []struct {
		name           string
		r              *RouteDestination
		expectedHost   string
		expectedWeight int32
		expectedPort   uint32
	}{
		{
			name: "Given a RouteDestination, " +
				"When destination is set to nil " +
				"And DeepCopyInto func is called" +
				"Then it should return a new RouteDestination with nil destination and different pointers",
			r: &RouteDestination{
				Destination: nil,
				Weight:      10,
				Headers: &networkingV1Alpha3.Headers{
					Request: &networkingV1Alpha3.Headers_HeaderOperations{},
				},
			},
			expectedWeight: 10,
			expectedPort:   8080,
		},
		{
			name: "Given a RouteDestination, " +
				"When header is set to nil " +
				"And DeepCopyInto func is called" +
				"Then it should return a new RouteDestination with nil headers and different pointers",
			r: &RouteDestination{
				Destination: &networkingV1Alpha3.Destination{
					Host: "host",
					Port: &networkingV1Alpha3.PortSelector{Number: 8080},
				},
				Weight:  10,
				Headers: nil,
			},
			expectedWeight: 10,
			expectedPort:   8080,
		},
		{
			name: "Given a RouteDestination, " +
				"When DeepCopyInto func is called" +
				"Then it should return a new RouteDestination with the same values and different pointers",
			r: &RouteDestination{
				Destination: &networkingV1Alpha3.Destination{
					Host: "host",
					Port: &networkingV1Alpha3.PortSelector{Number: 8080},
				},
				Weight: 10,
				Headers: &networkingV1Alpha3.Headers{
					Request: &networkingV1Alpha3.Headers_HeaderOperations{},
				},
			},
			expectedHost:   "host",
			expectedWeight: 10,
			expectedPort:   8080,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var newRouteDestination = &RouteDestination{}
			tc.r.DeepCopyInto(newRouteDestination)

			if tc.r.Destination != nil {
				if &tc.r.Destination == &newRouteDestination.Destination {
					require.Fail(t, "Expected new RouteDestination to have different pointers")
				}
				require.Equal(t, tc.r.Destination.Host, newRouteDestination.Destination.Host)
				require.Equal(t, tc.r.Destination.Port.Number, newRouteDestination.Destination.Port.Number)
			}
			if tc.r.Headers != nil {
				if &tc.r.Headers == &newRouteDestination.Headers {
					require.Fail(t, "Expected new RouteDestination to have different pointers")
				}
			}
			require.Equal(t, tc.r.Weight, newRouteDestination.Weight)
		})
	}

}
