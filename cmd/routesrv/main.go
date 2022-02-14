package main

import (
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/config"
	"github.com/zalando/skipper/routesrv"
)

func main() {
	cfg := config.NewConfig()
	cfg.Parse()
	log.SetLevel(cfg.ApplicationLogLevel)
	err := routesrv.Run(cfg.ToRouteSrvOptions())
	if err != nil {
		log.Fatal(err)
	}
}
