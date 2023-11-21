//go:build gofuzz
// +build gofuzz

package fuzz

import (
	"log"
	"net"

	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper"
	"github.com/zalando/skipper/config"
)

var initialized = false

func FuzzServer(data []byte) int {
	if !initialized {
		cfg := config.NewConfig()
		cfg.InlineRoutes = `r: * -> status(200) -> inlineContent("ok") -> <shunt>`
		cfg.ApplicationLogLevel = logrus.PanicLevel
		cfg.AccessLogDisabled = true
		cfg.ApplicationLog = "/dev/null"

		go func() {
			log.Fatal(skipper.Run(cfg.ToOptions()))
		}()

		initialized = true
	}

	conn, err := net.Dial("tcp", "localhost:9090")

	if err != nil {
		log.Printf("failed to dial: %v\n", err)
		return -1
	}

	conn.Write(data)
	conn.Close()

	return 1
}
