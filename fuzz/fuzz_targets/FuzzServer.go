//go:build gofuzz
// +build gofuzz

package fuzz

import (
	"errors"
	"log"
	"net"

	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper"
	"github.com/zalando/skipper/config"
)

var initialized = false

func connect(host string) (net.Conn, error) {
	for i := 0; i < 15; i++ {
		conn, err := net.Dial("tcp", host)

		if err != nil {
			continue
		}

		return conn, err
	}

	return nil, errors.New("unable to connect")
}

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

	conn, err := connect("localhost:9090")

	if err != nil {
		log.Printf("failed to dial: %v\n", err)
		return -1
	}

	conn.Write(data)
	conn.Close()

	return 1
}
