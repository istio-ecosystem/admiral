package routes

import (
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/clusters"
	"log"
	"net/http"
)

type RouteOpts struct {
	KubeconfigPath string
	RemoteRegistry *clusters.RemoteRegistry
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

}
