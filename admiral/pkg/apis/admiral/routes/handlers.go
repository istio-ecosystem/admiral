package routes

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/istio-ecosystem/admiral/admiral/pkg/clusters"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"log"
	"net/http"
	"strings"
)

type RouteOpts struct {
	KubeconfigPath string
	RemoteRegistry *clusters.RemoteRegistry
}

//type ClusterServiceEntries struct {
//	ServiceEntries []v1alpha3.ServiceEntry `json:"ServiceEntries,omitempty"`
//}

type IdentityServiceEntry struct {
	Cname        string   `json:"Cname,omitempty"`
	ClusterNames []string `json:"Clusters,omitempty"`
}

func (opts *RouteOpts) ReturnSuccessGET(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	response := fmt.Sprintf("Heath check method called: %v, URI: %v, Method: %v\n", r.Host, r.RequestURI, r.Method)

	_, writeErr := w.Write([]byte(response))
	if writeErr != nil {
		log.Printf("Error writing body: %v", writeErr)
		http.Error(w, "can't write body", http.StatusInternalServerError)
	}
}

func (opts *RouteOpts) GetClusters(w http.ResponseWriter, r *http.Request) {

	clusterList := []string{}

	// loop through secret controller's c.cs.remoteClusters to access all clusters admiral is watching
	for clusterID := range opts.RemoteRegistry.SecretController.Cs.RemoteClusters {
		clusterList = append(clusterList, clusterID)
	}

	out, err := json.Marshal(clusterList)
	if err != nil {
		log.Printf("Failed to marshall response for GetClusters call")
		http.Error(w, "Failed to marshall response", http.StatusInternalServerError)
	} else {
		if len(clusterList) == 0 {
			message := "No cluster is monitored by admiral"
			log.Printf(message)
			w.WriteHeader(200)
			out, _ = json.Marshal(message)
			w.Write(out)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write(out)
		}
	}
}

func (opts *RouteOpts) GetServiceEntriesByCluster(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	params := mux.Vars(r)
	clusterName := strings.Trim(params["clustername"], " ")

	var response []v1alpha3.ServiceEntry

	if clusterName != "" {

		serviceEntriesByCluster, err := clusters.GetServiceEntriesByCluster(clusterName, opts.RemoteRegistry)

		if err != nil {
			log.Printf(err.Error())
			if strings.Contains(err.Error(), "Admiral is not monitoring cluster") {
				http.Error(w, err.Error(), http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		} else {
			if len(serviceEntriesByCluster) == 0 {
				log.Printf(fmt.Sprintf("No service entries configured for cluster - %s", clusterName))
				w.WriteHeader(200)
				w.Write([]byte(fmt.Sprintf("No service entries configured for cluster - %s", clusterName)))
			} else {
				response = serviceEntriesByCluster
				out, err := json.Marshal(response)
				if err != nil {
					log.Printf("Failed to marshall response for GetServiceEntriesByCluster call")
					http.Error(w, fmt.Sprintf("Failed to marshall response for getting service entries api for cluster %s", clusterName), http.StatusInternalServerError)
				} else {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(200)
					w.Write(out)
				}
			}
		}
	} else {
		log.Printf("Cluster name not provided as part of the request")
		http.Error(w, "Cluster name not provided as part of the request", http.StatusBadRequest)
	}
}

func (opts *RouteOpts) GetServiceEntriesByIdentity(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	params := mux.Vars(r)
	identity := strings.Trim(params["identity"], " ")

	response := []IdentityServiceEntry{}

	if identity != "" {

		for cname, serviceCluster := range opts.RemoteRegistry.AdmiralCache.SeClusterCache.Map() {
			if strings.Contains(cname, identity) {
				var identityServiceEntry IdentityServiceEntry
				identityServiceEntry.Cname = cname
				for _, clusterId := range serviceCluster.Map() {
					identityServiceEntry.ClusterNames = append(identityServiceEntry.ClusterNames, clusterId)
				}
				response = append(response, identityServiceEntry)
			}
		}
		out, err := json.Marshal(response)
		if err != nil {
			log.Printf("Failed to marshall response GetServiceEntriesByIdentity call")
			http.Error(w, fmt.Sprintf("Failed to marshall response for getting service entries api for identity %s", identity), http.StatusInternalServerError)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write(out)
		}
	} else {
		log.Printf("Identity not provided as part of the request")
		http.Error(w, "Identity not provided as part of the request", http.StatusBadRequest)
	}
}
