package main

import (
	"github.com/zalando/skipper/config"
	"github.com/zalando/skipper/routesrv"
)

func main() {
	cfg := config.NewConfig()
	cfg.Parse()
	routesrv.Run(cfg.ToRouteSrvOptions())
}
