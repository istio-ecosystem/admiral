package server

import (
	"context"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"strconv"
)

type Service struct {
	Port   int
	ctx    context.Context
	server http.Server
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

func (s *Service) Start(port int, ctx context.Context, routes *Routes, filter []Filter) error {

	s.Port = port
	s.ctx = ctx

	go waitForStop(s)

	glog.Infof("Starting server on port=%d", port)
	router := s.newRouter(routes)

	s.server = http.Server{Addr: ":" + strconv.Itoa(port), Handler: router}

	return s.server.ListenAndServe()

}

func (s *Service) newRouter(routes *Routes) *mux.Router {

	router := mux.NewRouter().StrictSlash(true)
	for _, route := range *routes {

		var handler http.Handler
		handler = route.HandlerFunc

		for i := range route.FilterChain {

			//iterate backward through the slice to give a nature feel to defining the chain
			filter := route.FilterChain[len(route.FilterChain)-1-i]
			handler = filter.HandlerFunc(handler, route.Name)
		}

		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(handler)
		if route.Query != "" {
			router.Queries(route.Query)
		}
	}

	return router
}

func waitForStop(s *Service) {

	for {
		select {
		case <-s.ctx.Done():
			log.Println("context done stopping server")
			s.stop()
			return
		}
	}
}

func (s *Service) stop() error {

	return s.server.Close()

}
