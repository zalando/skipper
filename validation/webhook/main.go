package webhook

import (
	"errors"
	"net"
	"net/http"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/cmd/webhook/admission"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/validation"
)

type Runner struct {
	metrics metrics.Metrics
}

func NewRunner(mtr metrics.Metrics) *Runner {
	return &Runner{
		metrics: mtr,
	}
}

func (r *Runner) StartValidation(config validation.Config, filterRegistry filters.Registry, predicateSpecs []routing.PredicateSpec) error {
	log.Infof("Starting validation webhook server on %s", config.Address)

	mux := http.NewServeMux()

	rgAdmitter := &admission.RouteGroupAdmitter{
		RouteGroupValidator: &definitions.RouteGroupValidator{
			FilterRegistry: filterRegistry,
			PredicateSpecs: predicateSpecs,
			Metrics:        r.metrics,
		},
	}
	ingressAdmitter := &admission.IngressAdmitter{
		IngressValidator: &definitions.IngressV1Validator{
			FilterRegistry: filterRegistry,
			PredicateSpecs: predicateSpecs,
			Metrics:        r.metrics,
		},
	}

	mux.Handle("/routegroups", admission.Handler(rgAdmitter))
	mux.Handle("/ingresses", admission.Handler(ingressAdmitter))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    config.Address,
		Handler: mux,
	}

	if config.CertFile == "" || config.KeyFile == "" {
		log.Fatalf("validation webhook requires TLS: cert file or key file not provided")
		return errors.New("validation webhook requires TLS: cert file or key file not provided")
	}

	// Fail fast if the port cannot be bound, then serve TLS in a goroutine.
	ln, err := net.Listen("tcp", config.Address)
	if err != nil {
		log.Fatalf("failed to bind %s: %v", config.Address, err)
		return err
	}

	log.Infof("Starting HTTPS validation webhook server with cert: %s, key: %s", config.CertFile, config.KeyFile)

	if err = server.ServeTLS(ln, config.CertFile, config.KeyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("validation webhook server stopped: %v", err)
		return err
	}

	return nil
}
