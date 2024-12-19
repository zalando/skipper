package webhook

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/webhook/admission"
)

const (
	DefaultHTTPSAddress = ":9443"
	defaultHTTPAddress  = ":9080"
)

var DefaultLogLevel = log.InfoLevel.String()

type options struct {
	loglevel       string
	certFile       string
	keyFile        string
	address        string
	filterRegistry filters.Registry
}

func (opts *options) parse() {

	if opts.loglevel != "" {
		loglevel, err := log.ParseLevel(opts.loglevel)
		if err != nil {
			log.Error("Config parse error: ", err)
			log.SetLevel(log.InfoLevel)
		}
		log.SetLevel(loglevel)
	}

	if (opts.certFile != "" || opts.keyFile != "") && !(opts.certFile != "" && opts.keyFile != "") {
		log.Fatal("Config parse error: both of TLS cert & key must be provided or neither (for testing )")
		return
	}

	// support non-HTTPS for local testing
	if (opts.certFile == "" && opts.keyFile == "") && opts.address == DefaultHTTPSAddress {
		opts.address = defaultHTTPAddress
	}

}

func Run(loglevel, address, certFile, keyFile string, filterRegistry filters.Registry) {
	opts := &options{
		loglevel:       loglevel,
		address:        address,
		certFile:       certFile,
		keyFile:        keyFile,
		filterRegistry: filterRegistry,
	}
	run(opts)
}

func run(opts *options) {
	opts.parse()

	rgAdmitter := &admission.RouteGroupAdmitter{RouteGroupValidator: &definitions.RouteGroupValidator{FiltersRegistry: opts.filterRegistry}}
	ingressAdmitter := &admission.IngressAdmitter{IngressValidator: &definitions.IngressV1Validator{FiltersRegistry: opts.filterRegistry}}
	handler := http.NewServeMux()
	handler.Handle("/routegroups", admission.Handler(rgAdmitter))
	handler.Handle("/ingresses", admission.Handler(ingressAdmitter))
	handler.Handle("/metrics", promhttp.Handler())
	handler.HandleFunc("/healthz", healthCheck)

	// One can use generate_cert.go in https://golang.org/pkg/crypto/tls
	// to generate cert.pem and key.pem.
	serve(opts, handler)
}

func healthCheck(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write([]byte("ok")); err != nil {
		log.Errorf("Failed to write health check: %v", err)
	}

}

func serve(opts *options, handler http.Handler) {
	server := &http.Server{
		Addr:              opts.address,
		Handler:           handler,
		ReadTimeout:       1 * time.Minute,
		ReadHeaderTimeout: 1 * time.Minute,
	}

	log.Infof("Starting server on %s", opts.address)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)
	go func() {
		<-sig
		log.Info("Shutting down...")
		server.Shutdown(context.Background())
	}()

	var err error
	if opts.certFile != "" && opts.keyFile != "" {
		err = server.ListenAndServeTLS(opts.certFile, opts.keyFile)
	} else {
		// support non-HTTPS for local testing
		err = server.ListenAndServe()
	}

	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("Listener error: %v.", err)
	}
}
