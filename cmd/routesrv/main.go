package main

import (
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/config"
	"github.com/zalando/skipper/routesrv"
)

func main() {
	cfg := config.NewConfig()
	if err := cfg.Parse(); err != nil {
		log.Fatalf("Error processing config: %s", err)
	}

	log.SetLevel(cfg.ApplicationLogLevel)
	if cfg.ApplicationLogJSONEnabled {
		log.SetFormatter(&log.JSONFormatter{})
	}

	if err := routesrv.Run(cfg.ToOptions()); err != nil {
		log.Fatal(err)
	}
}
