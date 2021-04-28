package routes

import (
	"github.com/istio-ecosystem/admiral/pkg/filters"
	"github.com/istio-ecosystem/admiral/pkg/server"
	"k8s.io/client-go/tools/clientcmd"
	"log"
)

var Filter = server.Filters{
	server.Filter{HandlerFunc: filters.Logger},
}

func NewAdmiralAPIServer(opts *PaddleOpts) server.Routes {
	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", opts.KubeconfigPath)

	if err != nil || config == nil {
		log.Printf("could not retrieve kubeconfig: %v", err)
	}

	return server.Routes{

		server.Route{
			Name:        "Success GET",
			Method:      "GET",
			Pattern:     "/get",
			HandlerFunc: opts.ReturnSuccessGET,
		},
		server.Route{
			Name:        "Success health check",
			Method:      "GET",
			Pattern:     "/health/ready",
			HandlerFunc: opts.ReturnSuccessGET,
		},
	}

}
