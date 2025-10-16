package validation

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/admission"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

const (
	readTimeout         = time.Minute
	readHeaderTimeout   = time.Minute
	DefaultHTTPSAddress = ":9443"
	DefaultHTTPAddress  = ":9080"
)

// StartValidation launches the validation webhook server and keeps serving until the
// returned listener encounters an unrecoverable error, or the process shuts down.
func StartValidation(address, certFile, keyFile string, enableAdvancedValidation bool, filterRegistry filters.Registry, predicateSpecs []routing.PredicateSpec, mtr metrics.Metrics) error {

	if (certFile != "" || keyFile != "") && !(certFile != "" && keyFile != "") {
		err := errors.New("config parse error: both of TLS cert & key must be provided or neither (for testing)")
		log.Fatal(err)
		return err
	}

	// support non-HTTPS for local testing
	if (certFile == "" && keyFile == "") && address != DefaultHTTPAddress {
		address = DefaultHTTPAddress
	}

	handler := newValidationHandler(enableAdvancedValidation, filterRegistry, predicateSpecs, mtr)

	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	log.Infof("Starting server on %s", address)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)
	go func() {
		<-sig
		log.Info("Shutting down...")
		err := server.Shutdown(context.Background())
		if err != nil {
			return
		}
	}()
	var err error
	if certFile != "" && keyFile != "" {
		err = server.ListenAndServeTLS(certFile, keyFile)
	} else {
		// support non-HTTPS for local testing
		err = server.ListenAndServe()
	}

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Failed to listen: %v", err)
		return err
	}

	return nil
}

func newValidationHandler(enableAdvancedValidation bool, filterRegistry filters.Registry, predicateSpecs []routing.PredicateSpec, mtr metrics.Metrics) http.Handler {
	mux := http.NewServeMux()

	rgAdmitter := &admission.RouteGroupAdmitter{
		RouteGroupValidator: &definitions.RouteGroupValidator{
			FilterRegistry:           filterRegistry,
			PredicateSpecs:           predicateSpecs,
			Metrics:                  mtr,
			EnableAdvancedValidation: enableAdvancedValidation,
		},
	}

	ingressAdmitter := &admission.IngressAdmitter{
		IngressValidator: &definitions.IngressV1Validator{
			FilterRegistry:           filterRegistry,
			PredicateSpecs:           predicateSpecs,
			Metrics:                  mtr,
			EnableAdvancedValidation: enableAdvancedValidation,
		},
	}

	mux.Handle("/routegroups", admission.Handler(rgAdmitter))
	mux.Handle("/ingresses", admission.Handler(ingressAdmitter))
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", healthCheck)

	return mux
}

func healthCheck(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write([]byte("ok")); err != nil {
		log.Errorf("Failed to write health check: %v", err)
	}
}
