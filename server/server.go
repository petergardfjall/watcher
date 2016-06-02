package server

import (
	"github.com/gorilla/mux"
	"github.com/op/go-logging"

	"encoding/json"
	"fmt"
	"github.com/petergardfjall/watcher/engine"
	"net/http"
	"time"
)

var log = logging.MustGetLogger("server")

const (
	protocol = "https"
)

// A Server publishes information about a pinger Engine over HTTP.
type Server struct {
	engine     *engine.Engine
	httpServer *http.Server
	certFile   string
	keyFile    string
}

// NewServer creates a new Server running on a given port and publishing
// information about a given Engine.
func NewServer(engine *engine.Engine, port int, certFile, keyFile string) (*Server, error) {
	server := new(Server)
	server.engine = engine
	server.certFile = certFile
	server.keyFile = keyFile

	router := mux.NewRouter()
	router.Handle(
		"/pingers/", http.HandlerFunc(server.pingers)).
		Methods("GET")
	router.Handle(
		"/pingers/{name}", http.HandlerFunc(server.pingerStatus)).
		Methods("GET")
	router.Handle(
		"/pingers/{name}/output", http.HandlerFunc(server.pingerOutput)).
		Methods("GET")

	server.httpServer = &http.Server{
		Addr:        fmt.Sprintf(":%d", port),
		Handler:     router,
		ReadTimeout: 10 * time.Second,
		TLSConfig:   nil,
	}

	return server, nil
}

// Start starts a server and then blocks forever.
func (server *Server) Start() error {
	log.Debugf("starting engine ...")
	go server.engine.Start()
	log.Infof("starting server on %s ...", server.httpServer.Addr)
	return server.httpServer.ListenAndServeTLS(
		server.certFile, server.keyFile)
}

// pingers is a REST API endpoint that returns a list of pingers for the engine.
func (server *Server) pingers(w http.ResponseWriter, r *http.Request) {
	var pingerUrls []string
	for _, pinger := range server.engine.Pingers {
		url := fmt.Sprintf("%s://%s/pingers/%s", protocol, r.Host, pinger.Name)
		pingerUrls = append(pingerUrls, url)
	}

	respondWithJSON(w, r, pingerUrls)
}

// pingerStatus is a REST API endpoint that returns the current status of a
// given pinger.
func (server *Server) pingerStatus(w http.ResponseWriter, r *http.Request) {
	pathVars := mux.Vars(r)
	log.Debugf("getPingerStatus on %s", pathVars["name"])

	// verify that requested pinger exists
	pinger, ok := server.engine.Pingers[pathVars["name"]]
	if !ok {
		http.Error(w, fmt.Sprintf("%s: requested pinger does not exist", http.StatusText(http.StatusNotFound)), http.StatusNotFound)
		return
	}

	respondWithJSON(w, r, pinger.Status)

}

// pingerOuput is a REST API endpoint that returns the latest output returned
// by a given pinger (if any).
func (server *Server) pingerOutput(w http.ResponseWriter, r *http.Request) {
	pathVars := mux.Vars(r)
	log.Debugf("getPingerStatus on %s", pathVars["name"])

	// verify that requested pinger exists
	pinger, ok := server.engine.Pingers[pathVars["name"]]
	if !ok {
		http.Error(w, fmt.Sprintf("%s: requested pinger does not exist", http.StatusText(http.StatusNotFound)), http.StatusNotFound)
		return
	}

	if pinger.Output == nil {
		http.Error(w, fmt.Sprintf("%s: no output has been recorded (yet) for pinger", http.StatusText(http.StatusNotFound)), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	_, err := w.Write(pinger.Output.Bytes())
	if err != nil {
		log.Errorf("failed to write response on %s: %s", r.RequestURI, err)
	}

}

// Produces a JSON response to a HTTP request with a given object which is
// marshalled to json.
func respondWithJSON(w http.ResponseWriter, r *http.Request, object interface{}) {
	response, err := json.MarshalIndent(object, "", "    ")
	if err != nil {
		http.Error(w, fmt.Sprintf("%s: failed to marshal response: %s", http.StatusText(http.StatusInternalServerError), err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(response)
	if err != nil {
		log.Errorf("failed to write response on %s: %s", r.RequestURI, err)
	}
}
