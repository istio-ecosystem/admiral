package util

import (
	"fmt"
	proto "github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	networkingv1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"sort"
	"strings"
)

const defaultRoute = "default"

/*
 * Outlier detection issue.
 * Used for locality failover and circuit breaking...

 * within a source label set we need to match the order of the service client to make a deterministic object

 */

func Merge(sc *v1.ServiceClient, vs *v1alpha3.VirtualService, dr *v1alpha3.DestinationRule, sourceLabelKeys []string) (*v1alpha3.VirtualService, *v1alpha3.DestinationRule, error) {

	vs = vs.DeepCopy()
	dr = dr.DeepCopy()

	source, err := buildSourceLabels(sc, sourceLabelKeys)
	if err != nil {
		return nil, nil, err
	}

	//delete gen routes for this service client
	vs, err = deleteSourceRoutes(vs, source)
	if err != nil {
		return nil, nil, err
	}

	vsMatchGenMap, vsMatchMap := buildMatchMaps(vs, source)

	subset := make(map[string]*networkingv1alpha3.Subset)

	for _, ss := range dr.Spec.Subsets {
		if ss != nil {
			subset[ss.Name] = ss
		}
	}

	activeGeneratedRules := make([]string, 0)

	for _, http := range sc.Spec.Http {

		key, gen := buildMatchKey(http.Match, source)

		if gen {
			//this is a service client for the source labels should be blank make gen always false
			return nil, nil, fmt.Errorf("ServiceClient can't have source lables in the match")
		}

		//test make this idempotent we need to look rule matching the rule generated with the source generated label
		sourceGen := httpMatchWithSourceLabels(http.Match, source)
		keySourceGen, gen := buildMatchKey(sourceGen, source)

		if !gen {
			//this is a service client for the source labels should be blank make gen always false
			return nil, nil, fmt.Errorf("all source gen test http matches must be generated")
		}

		activeGeneratedRules = append(activeGeneratedRules, keySourceGen)

		route, gen := vsMatchGenMap[keySourceGen]
		if !gen {
			route2, ok := vsMatchMap[key]
			if !ok {
				if http.Match == nil {
					route, _ = vsMatchMap[defaultRoute]
				}
			} else {
				route = route2
			}
		}

		if route != nil {
			//exiting route clone it
			route = proto.Clone(route).(*networkingv1alpha3.HTTPRoute)
		} else {
			//exiting match didn't match mint a new route
			route = &networkingv1alpha3.HTTPRoute{}
			//route was null so no match was found so create a new match
			if http.Match != nil {
				route.Match = make([]*networkingv1alpha3.HTTPMatchRequest, 0)
				for _, m := range http.Match {
					route.Match = append(route.Match, proto.Clone(m).(*networkingv1alpha3.HTTPMatchRequest))
				}

			}

			defaultRoute, ok := vsMatchMap[defaultRoute]
			if ok {
				if defaultRoute.Route != nil {
					route.Route = make([]*networkingv1alpha3.HTTPRouteDestination, 0)
					for _, r := range defaultRoute.Route {
						route.Route = append(route.Route, proto.Clone(r).(*networkingv1alpha3.HTTPRouteDestination))
					}
				}
			}

		}

		if http.Timeout != nil {
			route.Timeout = &types.Duration{}
			s := http.Timeout.Seconds()
			route.Timeout.Seconds = int64(s)
		}

		if http.Fault != nil {
			route.Fault = proto.Clone(http.Fault).(*networkingv1alpha3.HTTPFaultInjection)
		}

		if http.Retries != nil {
			route.Retries = proto.Clone(http.Retries).(*networkingv1alpha3.HTTPRetry)
		}

		if !gen {
			//create settings
			//add source labels to match
			for _, m := range route.Match {
				m.SourceLabels = make(map[string]string)
				for key, value := range source {
					m.SourceLabels[key] = value
				}
			}

			prePendHTTPRoute(vs, route)

		}
	}

	return vs, dr, nil
}

func deleteSourceRoutes(vs *v1alpha3.VirtualService, source map[string]string) (*v1alpha3.VirtualService, error) {

	if len(source) == 0 {
		return nil, fmt.Errorf("source Map can't be empty")
	}

	routeToDelete := make([]int, 0)

	for i, r := range vs.Spec.Http {

		found := false
		for _, m := range r.Match {

			flag := true
			for k, v := range source {

				e, ok := m.SourceLabels[k]

				if ok {
					if e != v {
						flag = false
					}
				} else {
					flag = false
				}
			}
			found = found || flag
		}

		if found {
			routeToDelete = append(routeToDelete, i)
		}
	}

	for _, i := range routeToDelete {
		vs.Spec.Http[i] = nil
	}

	routes := make([]*networkingv1alpha3.HTTPRoute, 0)

	for _, r := range vs.Spec.Http {
		if r != nil {
			routes = append(routes, r)
		}
	}

	vs.Spec.Http = routes

	return vs, nil
}

//func deleteUnusedGeneratedRoutes(activeGeneratedRules []string, vs *v1alpha3.VirtualService, source map[string]string) {
//	activeGenMap := make(map[string]string)
//
//	for _, g := range activeGeneratedRules {
//		activeGenMap[g] = ""
//	}
//
//	routeToDelete := make([]int, 0)
//
//	for i, r := range vs.Spec.Http {
//		key, gen := buildMatchKey(r.Match, source)
//		if gen {
//			_, ok := activeGenMap[key]
//			if !ok {
//				//save for delete
//				routeToDelete = append(routeToDelete, i)
//			}
//		}
//	}
//
//	for _, i := range routeToDelete {
//		vs.Spec.Http[i] = nil
//	}
//
//	routes := make([]*networkingv1alpha3.HTTPRoute, 0)
//
//	for _, r := range vs.Spec.Http {
//		if r != nil {
//			routes = append(routes, r)
//		}
//	}
//
//	vs.Spec.Http = routes
//}

// This logic is needed for circuit breakers however it can work right now
// as those fields are needed for locality fail over.
//func outlierdetection(){
//if http.OutlierDetection != nil {
//
//	if route.Route == nil{
//
//		//no route one needs to be generate
//		genss := &networkingv1alpha3.Subset{
//			TrafficPolicy: &networkingv1alpha3.TrafficPolicy{},
//		}
//		//copy
//		genss.TrafficPolicy.OutlierDetection = proto.Clone(http.OutlierDetection).(*networkingv1alpha3.OutlierDetection)
//		genss.Name = "gen-blank-" + RandString(10)
//		//inset subset to dr
//		dr.Spec.Subsets = append(dr.Spec.Subsets, genss)
//
//		//sub to vs
//		route.Route = make([]*networkingv1alpha3.HTTPRouteDestination, 1)
//		rd := &networkingv1alpha3.HTTPRouteDestination{
//			Destination: &networkingv1alpha3.Destination{
//				Subset: genss.Name,
//			},
//		}
//		route.Route = append(route.Route, rd)
//	}else{
//
//		//collect the names
//		dnames := make(map[string] *networkingv1alpha3.Destination)
//		for _, d := range route.Route{
//			if d.Destination != nil{
//				dnames[d.Destination.Subset] = d.Destination
//			}
//		}
//
//		//two cases subset exist in dr or it doesn't
//		for n, v := range dnames{
//			ss := subset[n]
//			if ss == nil{
//				//no route one needs to be generate
//				genss := &networkingv1alpha3.Subset{
//					TrafficPolicy: &networkingv1alpha3.TrafficPolicy{},
//				}
//				genss.TrafficPolicy.OutlierDetection = proto.Clone(http.OutlierDetection).(*networkingv1alpha3.OutlierDetection)
//				genss.Name = "gen-" + n +"-" + RandString(10)
//				//inset subset to dr
//				dr.Spec.Subsets = append(dr.Spec.Subsets, genss)
//				//set the new gen name
//				dnames[n].Subset = genss.Name
//
//			}else{
//				//if name starts with gen copy and replace
//				//if it doesn't copy replace outlier add gen name add to vs and dr
//				newSubSet := proto.Clone(ss).(*networkingv1alpha3.Subset)
//				if newSubSet.TrafficPolicy == nil{
//					newSubSet.TrafficPolicy = &networkingv1alpha3.TrafficPolicy{}
//				}
//				newSubSet.TrafficPolicy.OutlierDetection = proto.Clone(http.OutlierDetection).(*networkingv1alpha3.OutlierDetection)
//
//				if !strings.HasPrefix("gen", n){
//					newSubSet.Name = "gen-" + n +"-" + RandString(10)
//					dr.Spec.Subsets = append(dr.Spec.Subsets, newSubSet)
//					v.Subset = newSubSet.Name
//				}
//			}
//		}
//	}
//}
//}

func prePendHTTPRoute(vs *v1alpha3.VirtualService, route *networkingv1alpha3.HTTPRoute) {
	vs.Spec.Http = append(vs.Spec.Http, &networkingv1alpha3.HTTPRoute{})
	copy(vs.Spec.Http[1:], vs.Spec.Http)
	vs.Spec.Http[0] = route
}

//first return map was flagged as generated seconds was not
func buildMatchMaps(vs *v1alpha3.VirtualService, source map[string]string) (map[string]*networkingv1alpha3.HTTPRoute, map[string]*networkingv1alpha3.HTTPRoute) {
	//map used to look up exiting matches that have not been generated
	vsMatchMap := make(map[string]*networkingv1alpha3.HTTPRoute, len(vs.Spec.Http))

	//map used to look up exiting matches that have been generated
	//if the source label match is equal to this client service
	//it will be considered generated by the is client service
	vsMatchGenMap := make(map[string]*networkingv1alpha3.HTTPRoute, len(vs.Spec.Http))

	for _, http := range vs.Spec.Http {
		if http.Match != nil {
			compositeKey, gen := buildMatchKey(http.Match, source)
			if gen {
				vsMatchGenMap[compositeKey] = http
			} else {
				vsMatchMap[compositeKey] = http
			}
		} else {
			vsMatchMap[defaultRoute] = http
		}
	}

	return vsMatchGenMap, vsMatchMap
}

func buildSourceLabels(sc *v1.ServiceClient, sourceLabelKeys []string) (map[string]string, error) {
	source := make(map[string]string, len(sourceLabelKeys))

	//pre source label match
	for _, key := range sourceLabelKeys {
		value, ok := sc.Labels[key]
		if !ok {
			return nil, fmt.Errorf("failed to match lables sourcekeys: %v    service client map: %v", sourceLabelKeys, sc.Labels)
		}
		source[key] = value
	}
	return source, nil
}

func buildMatchKey(match []*networkingv1alpha3.HTTPMatchRequest, sourceLabels map[string]string) (string, bool) {

	gen := false
	keys := make([]string, len(match))
	for _, m := range match {

		gen = testGenSourceLabels(m, sourceLabels, gen)
		keys = append(keys, m.String())
	}

	sort.Strings(keys)
	compositKey := strings.Join(keys, "-")
	return compositKey, gen
}

func testGenSourceLabels(match *networkingv1alpha3.HTTPMatchRequest, sourceLabels map[string]string, gen bool) bool {
	if match.SourceLabels != nil {
		test := true

		//if the labels don't match return gen false
		for key, value := range match.SourceLabels {

			v, ok := sourceLabels[key]
			if ok {
				if v != value {
					test = false
				}
			} else {
				test = false
			}
		}
		gen = test
	}
	return gen
}

func httpMatchWithSourceLabels(match []*networkingv1alpha3.HTTPMatchRequest, sourceLabel map[string]string) []*networkingv1alpha3.HTTPMatchRequest {

	res := make([]*networkingv1alpha3.HTTPMatchRequest, 0)
	for _, m := range match {
		newMatch := proto.Clone(m).(*networkingv1alpha3.HTTPMatchRequest)
		newMatch.SourceLabels = sourceLabel
		res = append(res, newMatch)
	}
	return res
}
