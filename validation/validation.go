// Package validation provides Kubernetes validation webhook related code.
package validation

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/admission"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/routing"
)

const (
	readTimeout       = time.Minute
	readHeaderTimeout = time.Minute
)

type Options struct {
	Address                  string
	CertFile                 string
	KeyFile                  string
	EnableAdvancedValidation bool
}

// StartValidation launches the validation webhook server and keeps serving until the
// returned listener encounters an unrecoverable error, or the process shuts down.
func StartValidation(opts Options, routingOptions routing.Options) error {
	if opts.CertFile == "" || opts.KeyFile == "" {
		return errors.New("validation webhook requires TLS: cert file or key file not provided")
	}

	server := &http.Server{
		Addr:              opts.Address,
		Handler:           newValidationHandler(opts.EnableAdvancedValidation, routingOptions),
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	log.Infof("Starting server on %s", opts.Address)

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

	err := server.ListenAndServeTLS(opts.CertFile, opts.KeyFile)

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("failed to listen: %w", err)
	}

	return nil
}

func newValidationHandler(enableAdvancedValidation bool, routingOptions routing.Options) http.Handler {
	mux := http.NewServeMux()

	rgAdmitter := &admission.RouteGroupAdmitter{
		RouteGroupValidator: &definitions.RouteGroupValidator{
			RoutingOptions:           routingOptions,
			EnableAdvancedValidation: enableAdvancedValidation,
		},
	}

	ingressAdmitter := &admission.IngressAdmitter{
		IngressValidator: &definitions.IngressV1Validator{
			RoutingOptions:           routingOptions,
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
