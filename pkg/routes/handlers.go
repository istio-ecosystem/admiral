package routes

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

var charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

type PaddleOpts struct {
	ReturnEndpoint   string
	ContinuePingPong bool
	AppId            string
	AppSecret        string
	UsingGW          bool
	Delay            int
	KubeconfigPath   string
}

func (opts *PaddleOpts) ReturnSuccessGET(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)

	response := handleDelayOrInjectPayload(r)
	_, writeErr := w.Write([]byte(response))
	if writeErr != nil {
		log.Printf("Error writing body: %v", writeErr)
		http.Error(w, "can't write body", http.StatusInternalServerError)
	}
}

func handleDelayOrInjectPayload(request *http.Request) string {

	query := request.URL.Query()

	delayInSec, _ := strconv.Atoi(query.Get("delay"))

	if delayInSec > 0 {
		time.Sleep(time.Duration(delayInSec) * time.Second)
	}

	payloadInBytes, _ := strconv.Atoi(query.Get("payload"))
	if payloadInBytes > 0 {
		token := make([]byte, payloadInBytes*1)
		for i := range token {
			token[i] = byte(charset[rand.Intn(len(charset))])
		}
		return string(token)
	}
	return fmt.Sprintf("Backend host: %v, URI: %v, Method: %v", request.Host, request.RequestURI, request.Method)
}