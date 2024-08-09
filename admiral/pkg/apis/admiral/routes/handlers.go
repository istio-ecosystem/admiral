package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	commonUtil "github.com/istio-ecosystem/admiral/admiral/pkg/util"

	"github.com/gorilla/mux"
	"github.com/istio-ecosystem/admiral/admiral/pkg/clusters"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/sirupsen/logrus"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
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

/*
We expect the DNS health checker to include the query param checkifreadonly with value set to true.
The query param is used to check if the current Admiral instance is running in Active Mode or Passive Mode (also called read only mode).
If Running in passive  mode, the health check returns 502 which forces DNS lookup to always return reference to Admiral in Active state.
*/

func (opts *RouteOpts) ReturnSuccessGET(w http.ResponseWriter, r *http.Request) {
	allQueryParams := r.URL.Query()
	checkIfReadOnlyStringVal := allQueryParams.Get("checkifreadonly")
	//Remove all spaces
	checkIfReadOnlyStringVal = strings.ReplaceAll(checkIfReadOnlyStringVal, " ", "")
	// checkIfReadOnlyStringVal will be empty in case ""checkifreadonly" query param is not sent in the request. checkIfReadOnlyBoolVal will be false
	checkIfReadOnlyBoolVal, err := strconv.ParseBool(checkIfReadOnlyStringVal)
	var response string

	if len(checkIfReadOnlyStringVal) == 0 || nil == err {
		if checkIfReadOnlyBoolVal {

			if commonUtil.IsAdmiralReadOnly() {
				//Force fail health check if Admiral is in Readonly mode
				w.WriteHeader(503)
			} else {
				w.WriteHeader(200)
			}
		} else {
			w.WriteHeader(200)
		}
		response = fmt.Sprintf("Heath check method called: %v, URI: %v, Method: %v\n", r.Host, r.RequestURI, r.Method)
	} else {
		w.WriteHeader(400)
		response = fmt.Sprintf("Health check method called with bad query param value %v for checkifreadonly", checkIfReadOnlyStringVal)
	}
	_, writeErr := w.Write([]byte(response))
	if writeErr != nil {
		logrus.Printf("Error writing body: %v", writeErr)
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
		logrus.Printf("Failed to marshall response for GetClusters call")
		http.Error(w, "Failed to marshall response", http.StatusInternalServerError)
	} else {
		if len(clusterList) == 0 {
			message := "No cluster is monitored by admiral"
			logrus.Println(message)
			w.WriteHeader(200)
			out, _ = json.Marshal(message)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
		}
		_, err := w.Write(out)
		if err != nil {
			logrus.Println("Failed to write message: ", err)
		}
	}
}

func (opts *RouteOpts) GetServiceEntriesByCluster(w http.ResponseWriter, r *http.Request) {
	ctxLogger := logrus.WithFields(logrus.Fields{
		"txId": common.FetchTxIdOrGenNew(context.TODO()),
	})
	defer r.Body.Close()
	params := mux.Vars(r)
	clusterName := strings.Trim(params["clustername"], " ")

	var response []*v1alpha3.ServiceEntry

	ctx := r.Context()

	if clusterName != "" {

		serviceEntriesByCluster, err := clusters.GetServiceEntriesByCluster(ctxLogger, ctx, clusterName, opts.RemoteRegistry)

		if err != nil {
			logrus.Printf("API call get service entry by cluster failed for clustername %v with Error: %v", clusterName, err.Error())
			if strings.Contains(strings.ToLower(err.Error()), strings.ToLower("Admiral is not monitoring cluster")) {
				http.Error(w, err.Error(), http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		} else {
			if len(serviceEntriesByCluster) == 0 {
				logrus.Printf("API call get service entry by cluster failed for clustername %v with Error: %v", clusterName, "no service entries configured for cluster - "+clusterName)
				w.WriteHeader(200)
				_, err := w.Write([]byte(fmt.Sprintf("no service entries configured for cluster - %s", clusterName)))
				if err != nil {
					logrus.Println("Error writing body: ", err)
				}

			} else {
				response = serviceEntriesByCluster
				out, err := json.Marshal(response)
				if err != nil {
					logrus.Printf("Failed to marshall response for GetServiceEntriesByCluster call")
					http.Error(w, fmt.Sprintf("Failed to marshall response for getting service entries api for cluster %s", clusterName), http.StatusInternalServerError)
				} else {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(200)
					_, err := w.Write(out)
					if err != nil {
						logrus.Println("failed to write resp body: ", err)
					}
				}
			}
		}
	} else {
		logrus.Printf("cluster name not provided as part of the request")
		http.Error(w, "cluster name not provided as part of the request", http.StatusBadRequest)
	}
}

// GetGlobalTrafficPolicyByIdentityAndEnv handler returns GlobalTrafficPolicy resource based on
// the matching env and identity passed as query parameters
func (opts *RouteOpts) GetGlobalTrafficPolicyByIdentityAndEnv(w http.ResponseWriter, r *http.Request) {

	pathParams := mux.Vars(r)
	identity, ok := pathParams["identity"]
	if !ok || identity == "" {
		generateErrorResponse(w, http.StatusBadRequest, "identity not provided as part of the path param")
		return
	}

	env := r.FormValue("env")
	if env == "" {
		env = "default"
	}

	if opts.RemoteRegistry == nil || opts.RemoteRegistry.AdmiralCache == nil {
		logrus.Warn("invalid remote registry cache")
		generateErrorResponse(w, http.StatusInternalServerError, "invalid remote registry cache")
		return
	}

	gtps := opts.RemoteRegistry.AdmiralCache.GlobalTrafficCache

	if gtps == nil {
		logrus.Print("globaltrafficcache not initialized")
		generateErrorResponse(w, http.StatusInternalServerError, "invalid globaltrafficcache")
		return
	}

	gtp, err := gtps.GetFromIdentity(identity, env)
	if err != nil {
		logrus.Warn(err)
		generateErrorResponse(w, http.StatusInternalServerError, err.Error())
	}
	if gtp == nil {
		generateErrorResponse(w, http.StatusNotFound, fmt.Sprintf("globaltraffic policy with identity: %s and env: %s was not found", identity, env))
		return
	}

	generateResponseJSON(w, http.StatusOK, gtp)
}

func (opts *RouteOpts) GetServiceEntriesByIdentity(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	params := mux.Vars(r)
	identity := strings.Trim(params["identity"], " ")

	response := []IdentityServiceEntry{}

	if identity != "" {

		m := opts.RemoteRegistry.AdmiralCache.SeClusterCache

		m.Range(func(cname string, serviceCluster *common.Map) {
			if strings.Contains(cname, identity) {
				var identityServiceEntry IdentityServiceEntry
				identityServiceEntry.Cname = cname
				serviceCluster.Range(func(k string, clusterID string) {
					identityServiceEntry.ClusterNames = append(identityServiceEntry.ClusterNames, clusterID)
				})
				response = append(response, identityServiceEntry)
			}
		})

		out, err := json.Marshal(response)
		if err != nil {
			logrus.Printf("Failed to marshall response GetServiceEntriesByIdentity call")
			http.Error(w, fmt.Sprintf("Failed to marshall response for getting service entries api for identity %s", identity), http.StatusInternalServerError)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, err := w.Write(out)
			if err != nil {
				logrus.Println("failed to write resp body", err)
			}
		}
	} else {
		logrus.Printf("Identity not provided as part of the request")
		http.Error(w, "Identity not provided as part of the request", http.StatusBadRequest)
	}
}

func generateErrorResponse(w http.ResponseWriter, code int, message string) {
	generateResponseJSON(w, code, map[string]string{"error": message})
}

func generateResponseJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		logrus.Printf("failed to serialize the payload due to %v", err)
		response = []byte("{\"error\": \"malformed response payload\"}")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}
