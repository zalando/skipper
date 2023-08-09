package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/cmd/webhook/admission"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
)

const (
	defaultHTTPSAddress = ":9443"
	defaultHTTPAddress  = ":9080"
)

type config struct {
	debug    bool
	certFile string
	keyFile  string
	address  string
}

func (c *config) parse() {
	flag.BoolVar(&c.debug, "debug", false, "Enable debug logging")
	flag.StringVar(&c.certFile, "tls-cert-file", os.Getenv("CERT_FILE"), "File containing the certificate for HTTPS")
	flag.StringVar(&c.keyFile, "tls-key-file", os.Getenv("KEY_FILE"), "File containing the private key for HTTPS")
	flag.StringVar(&c.address, "address", defaultHTTPSAddress, "The address to listen on")
	flag.Parse()

	if (c.certFile != "" || c.keyFile != "") && !(c.certFile != "" && c.keyFile != "") {
		log.Fatal("Config parse error: both of TLS cert & key must be provided or neither (for testing )")
		return
	}

	// support non-HTTPS for local testing
	if (c.certFile == "" && c.keyFile == "") && c.address == defaultHTTPSAddress {
		c.address = defaultHTTPAddress
	}

	if c.debug {
		log.SetLevel(log.DebugLevel)
	}
}

func main() {
	var cfg = &config{}
	cfg.parse()

	rgAdmitter := &admission.RouteGroupAdmitter{RouteGroupValidator: &definitions.RouteGroupValidator{}}
	ingressAdmitter := &admission.IngressAdmitter{IngressValidator: &definitions.IngressV1Validator{}}
	handler := http.NewServeMux()
	handler.Handle("/routegroups", admission.Handler(rgAdmitter))
	handler.Handle("/ingresses", admission.Handler(ingressAdmitter))
	handler.Handle("/metrics", promhttp.Handler())
	handler.HandleFunc("/healthz", healthCheck)

	// One can use generate_cert.go in https://golang.org/pkg/crypto/tls
	// to generate cert.pem and key.pem.
	serve(cfg, handler)
}

func healthCheck(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write([]byte("ok")); err != nil {
		log.Errorf("Failed to write health check: %v", err)
	}

}

func serve(cfg *config, handler http.Handler) {
	server := &http.Server{
		Addr:              cfg.address,
		Handler:           handler,
		ReadTimeout:       1 * time.Minute,
		ReadHeaderTimeout: 1 * time.Minute,
	}

	log.Infof("Starting server on %s", cfg.address)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)
	go func() {
		<-sig
		log.Info("Shutting down...")
		server.Shutdown(context.Background())
	}()

	var err error
	if cfg.certFile != "" && cfg.keyFile != "" {
		err = server.ListenAndServeTLS(cfg.certFile, cfg.keyFile)
	} else {
		// support non-HTTPS for local testing
		err = server.ListenAndServe()
	}

	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("Listener error: %v.", err)
	}
}
