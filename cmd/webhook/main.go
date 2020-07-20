package main

import (
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/zalando/skipper/cmd/webhook/admission"
)

func main() {

	http.HandleFunc("/healthz", healthCheck)
	http.HandleFunc("/routegroups", admission.Handler)
	port := ":8080"
	log.Infof("Listening on port %s ...", port)
	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Fatal(err)
	}
}

func healthCheck(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write([]byte("ok")); err != nil {
		log.Error("failed to write health check: %v", err)
	}

}
