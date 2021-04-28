package filters

import (
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

/*
 * func for the "middleware" or filters for example Access logging or processing Authn/z
 */

func Logger(inner http.Handler, name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		inner.ServeHTTP(w, r)
		body, _ := ioutil.ReadAll(r.Body)

		log.Printf(
			"Access Logger %s\t%s\t%s\t%s\t%s",
			r.Method,
			r.RequestURI,
			name,
			time.Since(start),
			string(body),
		)
	})
}


func Auth(inner http.Handler, name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		log.Printf(
			"Auth Logger for endpoint %s", name,
		)
		//TODO implement authnz

		inner.ServeHTTP(w, r)

	})
}
