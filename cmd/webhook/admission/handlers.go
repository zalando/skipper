package admission

import (
	"net/http"

	log "github.com/sirupsen/logrus"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
		// TODO: inc prometheus invalid req counter
		w.WriteHeader(http.StatusBadRequest)
	}

	if _, err := w.Write([]byte("ok")); err != nil {
		log.Errorf("unable to write response: %v", err)
	}
}
