package validation

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/admission"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/routing"
)

type Runner struct {
	metrics  metrics.Metrics
	serverMu sync.Mutex
	server   *http.Server
}

func NewRunner(mtr metrics.Metrics) *Runner {
	return &Runner{
		metrics: mtr,
	}
}

func (r *Runner) StartValidation(address string, certFile string, keyFile string, filterRegistry filters.Registry, predicateSpecs []routing.PredicateSpec) error {
	log.Infof("Starting validation webhook server on %s", address)

	mux := http.NewServeMux()

	rgAdmitter := &admission.RouteGroupAdmitter{
		RouteGroupValidator: &definitions.RouteGroupValidator{
			FilterRegistry:          filterRegistry,
			PredicateSpecs:          predicateSpecs,
			Metrics:                 r.metrics,
			EnableWebhookValidation: true,
		},
	}
	ingressAdmitter := &admission.IngressAdmitter{
		IngressValidator: &definitions.IngressV1Validator{
			FilterRegistry:          filterRegistry,
			PredicateSpecs:          predicateSpecs,
			Metrics:                 r.metrics,
			EnableWebhookValidation: true,
		},
	}

	mux.Handle("/routegroups", admission.Handler(rgAdmitter))
	mux.Handle("/ingresses", admission.Handler(ingressAdmitter))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    address,
		Handler: mux,
	}

	if !r.setServer(server) {
		return errors.New("validation webhook server already running")
	}

	if certFile == "" || keyFile == "" {
		r.clearServer(server)
		log.Fatalf("validation webhook requires TLS: cert file or key file not provided")
		return errors.New("validation webhook requires TLS: cert file or key file not provided")
	}

	// Fail fast if the port cannot be bound, then serve TLS in a goroutine.
	ln, err := net.Listen("tcp", address)
	if err != nil {
		r.clearServer(server)
		log.Fatalf("failed to bind %s: %v", address, err)
		return err
	}

	log.Infof("Starting HTTPS validation webhook server with cert: %s, key: %s", certFile, keyFile)

	go func() {
		defer r.clearServer(server)
		if serveErr := server.ServeTLS(ln, certFile, keyFile); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Fatalf("validation webhook server stopped: %v", serveErr)
		}
	}()

	return nil
}

// Stop Gives the test a clean way to stop the web server after each case, avoid port leaks,
// and allow repeated start/stop cycles
func (r *Runner) Stop(ctx context.Context) error {
	srv := r.swapServer(nil)
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

func (r *Runner) setServer(server *http.Server) bool {
	r.serverMu.Lock()
	defer r.serverMu.Unlock()
	if r.server != nil {
		return false
	}
	r.server = server
	return true
}

func (r *Runner) clearServer(server *http.Server) {
	r.serverMu.Lock()
	if r.server == server {
		r.server = nil
	}
	r.serverMu.Unlock()
}

func (r *Runner) swapServer(server *http.Server) *http.Server {
	r.serverMu.Lock()
	old := r.server
	r.server = server
	r.serverMu.Unlock()
	return old
}
