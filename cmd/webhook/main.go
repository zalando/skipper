package main

import (
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/zalando/skipper/cmd/webhook/admission"
)

const (
	defaultAddress = ":8080"
)

func main() {
	var (
		certFile string
		keyFile  string
		address  string
	)

	kingpin.Flag("tls-cert-file", "File containing the certificate for HTTPS").Required().Envar("CERT_FILE").StringVar(&certFile)
	kingpin.Flag("tls-key-file", "File containing the private key for HTTPS").Required().Envar("KEY_FILE").StringVar(&keyFile)
	kingpin.Flag("address", "The address to listen on").Default(defaultAddress).StringVar(&address)

	kingpin.Parse()

	rgAdmitter := admission.RouteGroupAdmitter{}
	handler := http.NewServeMux()
	handler.Handle("/routegroups", admission.Handler(rgAdmitter))
	handler.Handle("/metrics", promhttp.Handler())
	handler.HandleFunc("/healthz", healthCheck)

	// One can use generate_cert.go in https://golang.org/pkg/crypto/tls
	// to generate cert.pem and key.pem.
	serve(address, certFile, keyFile, handler)
}

func healthCheck(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write([]byte("ok")); err != nil {
		log.Error("failed to write health check: %v", err)
	}

}

func serve(address, certFile, keyFile string, handler http.Handler) {
	server := &http.Server{
		Addr:    address,
		Handler: handler,
	}

	log.Infof("Starting server on %s", address)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)
	go func() {
		<-sig
		server.Shutdown(context.Background())
	}()

	err := server.ListenAndServeTLS(certFile, keyFile)
	if err != nil {
		if err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}
}
