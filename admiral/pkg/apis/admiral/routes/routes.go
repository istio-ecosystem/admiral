package routes

import (
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/filters"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/server"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"net/http"
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
	}
}

func NewMetricsServer() server.Routes {

	if common.GetMetricsEnabled() {
		return server.Routes{
			server.Route{
				Name:        "Get metrics in prometheus format",
				Method:      "GET",
				Pattern:     "/metrics",
				HandlerFunc: promhttp.Handler().ServeHTTP,
			},
		}
	}
	return server.Routes{
		server.Route{
			Name:        "Noop metrics",
			Method:      "GET",
			Pattern:     "/metrics",
			HandlerFunc: Noop,
		},
	}
}

func Noop(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
