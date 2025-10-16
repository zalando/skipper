package main

import (
	"flag"
	"os"

	"github.com/zalando/skipper"

	log "github.com/sirupsen/logrus"
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

	if c.debug {
		log.SetLevel(log.DebugLevel)
	}
}

func main() {
	var cfg = &config{}
	cfg.parse()

	skpOptions := skipper.Options{
		ValidationWebhookEnabled:  true,
		ValidationWebhookCertFile: cfg.certFile,
		ValidationWebhookKeyFile:  cfg.keyFile,
		ValidationWebhookAddress:  cfg.address,
		EnableAdvancedValidation:  false,
	}

	if err := skipper.Run(skpOptions); err != nil {
		log.Fatalf("Failed to start skipper binary in validation mode %v", err)
		return
	}

}
