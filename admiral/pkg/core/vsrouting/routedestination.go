package vsrouting

import (
	networkingV1Alpha3 "istio.io/api/networking/v1alpha3"
)

type IRouteDestination interface {
	ToTLSRouteDestination() *networkingV1Alpha3.RouteDestination
	ToHTTPRouteDestination() *networkingV1Alpha3.HTTPRouteDestination
	DeepCopyInto(*RouteDestination)
}

type RouteDestination struct {
	Destination *networkingV1Alpha3.Destination
	Weight      int32
	Headers     *networkingV1Alpha3.Headers
	Type        string
}

type RouteDestinationSorted []*RouteDestination

func (r *RouteDestination) ToTLSRouteDestination() *networkingV1Alpha3.RouteDestination {
	return &networkingV1Alpha3.RouteDestination{
		Destination: r.Destination,
		Weight:      r.Weight,
	}
}

func (r *RouteDestination) ToHTTPRouteDestination() *networkingV1Alpha3.HTTPRouteDestination {
	return &networkingV1Alpha3.HTTPRouteDestination{
		Destination: r.Destination,
		Weight:      r.Weight,
		Headers:     r.Headers,
	}
}

func (r *RouteDestination) DeepCopyInto(out *RouteDestination) {
	var newDestination = &networkingV1Alpha3.Destination{}
	if r.Destination != nil {
		r.Destination.DeepCopyInto(newDestination)
	}
	var newHeaders = &networkingV1Alpha3.Headers{}
	if r.Headers != nil {
		r.Headers.DeepCopyInto(newHeaders)
	}

	out.Destination = newDestination
	out.Weight = r.Weight
	out.Headers = newHeaders
}

func (r RouteDestinationSorted) Len() int {
	return len(r)
}

func (r RouteDestinationSorted) Less(i, j int) bool {
	return r[i].Destination.Host < r[j].Destination.Host
}

func (r RouteDestinationSorted) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}
