package server

import (
	"context"
	"github.com/gorilla/mux"
	"github.com/istio-ecosystem/admiral/admiral/pkg/clusters"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type Service struct {
	Port           int
	ctx            context.Context
	server         http.Server
	remoteRegistry *clusters.RemoteRegistry
}

// filter definition as a func
type FilterHandlerFunc func(inner http.Handler, name string) http.Handler

//structs used to collect routes and filters
type Filter struct {
	HandlerFunc FilterHandlerFunc
}

type Route struct {
	Name        string
	Method      string
	Pattern     string
	Query       string
	HandlerFunc http.HandlerFunc
	FilterChain Filters
}

type Routes []Route

type Filters []Filter

func (s *Service) Start(ctx context.Context, port int, routes Routes, filter []Filter, remoteRegistry *clusters.RemoteRegistry) {

	s.ctx = ctx
	s.Port = port
	s.remoteRegistry = remoteRegistry

	go waitForStop(s)

	router := s.newRouter(routes, filter)

	s.server = http.Server{Addr: ":" + strconv.Itoa(port), Handler: router}

	log.Printf("Starting server on port=%d", port)
	log.Fatalln(s.server.ListenAndServe())

}

func (s *Service) newRouter(routes Routes, filter []Filter) *mux.Router {

	router := mux.NewRouter().StrictSlash(true)
	for _, route := range routes {

		var handler http.Handler
		handler = route.HandlerFunc

		for _, filter := range filter {
			handler = filter.HandlerFunc(handler, route.Name)
		}

		router.Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(handler)

		if route.Query != "" {
			queries := strings.Split(route.Query, ",")
			for _, query := range queries {
				router.Queries(query, "{query}")
			}
		}
	}

	return router
}

func waitForStop(s *Service) {
	for range s.ctx.Done() {
		log.Println("context done stopping server")
		err := s.stop()
		if err != nil {
			log.Println("error stopping server: ", err)
		}
	}
}

func (s *Service) stop() error {
	return s.server.Close()
}
