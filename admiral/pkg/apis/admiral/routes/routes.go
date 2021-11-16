package routes

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/filters"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/server"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"net/http/pprof"
)

var Filter = server.Filters{
	server.Filter{HandlerFunc: filters.Logger},
}

func NewAdmiralAPIServer(opts *RouteOpts) server.Routes {
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", opts.KubeconfigPath)

	if err != nil || config == nil {
		log.Printf("could not retrieve kubeconfig: %v", err)
	}

	return server.Routes{

		server.Route{
			Name:        "Success health check",
			Method:      "GET",
			Pattern:     "/health/ready",
			HandlerFunc: opts.ReturnSuccessGET,
		},
		server.Route{
			Name:        "Get list of clusters admiral is watching",
			Method:      "GET",
			Pattern:     "/clusters",
			HandlerFunc: opts.GetClusters,
		},
		server.Route{
			Name:        "Get list service entries for a given cluster",
			Method:      "GET",
			Pattern:     "/cluster/{clustername}/serviceentries",
			HandlerFunc: opts.GetServiceEntriesByCluster,
		},
		server.Route{
			Name:        "Get list service entries for a given identity",
			Method:      "GET",
			Pattern:     "/identity/{identity}/serviceentries",
			HandlerFunc: opts.GetServiceEntriesByIdentity,
		},
		server.Route{
			Name:        "Debug Index",
			Method:      "GET",
			Pattern:     "/debug/pprof",
			HandlerFunc: pprof.Index,
		},
		server.Route{
			Name:        "Debug CLI",
			Method:      "GET",
			Pattern:     "/debug/pprof/cmdline",
			HandlerFunc: pprof.Cmdline,
		},
		server.Route{
			Name:        "Debug Profile",
			Method:      "GET",
			Pattern:     "/debug/pprof/profile",
			HandlerFunc: pprof.Profile,
		},
		server.Route{
			Name:        "Debug Symbol",
			Method:      "GET",
			Pattern:     "/debug/pprof/symbol",
			HandlerFunc: pprof.Symbol,
		},
		server.Route{
			Name:        "Debug Trace",
			Method:      "GET",
			Pattern:     "/debug/pprof/trace",
			HandlerFunc: pprof.Trace,
		},
	}
}
