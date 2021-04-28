package routes

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/filters"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/server"
	"k8s.io/client-go/tools/clientcmd"
	"log"
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
	}
}
