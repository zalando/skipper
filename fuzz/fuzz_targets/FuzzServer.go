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

var (
	initialized = false
	address     = ""
)

func findAddress() (string, error) {
	l, err := net.ListenTCP("tcp6", &net.TCPAddr{})

	if err != nil {
		return "", err
	}

	defer l.Close()

	return l.Addr().String(), nil
}

func connect(host string) (net.Conn, error) {
	for i := 0; i < 15; i++ {
		conn, err := net.Dial("tcp6", host)

		if err != nil {
			continue
		}

		return conn, err
	}

	return nil, errors.New("unable to connect")
}

func FuzzServer(data []byte) int {
	if !initialized {
		addr, err := findAddress()

		if err != nil {
			log.Printf("failed to find address: %v\n", err)
			return -1
		}

		cfg := config.NewConfig()
		cfg.InlineRoutes = `r: * -> status(200) -> inlineContent("ok") -> <shunt>`
		cfg.ApplicationLogLevel = logrus.PanicLevel
		cfg.AccessLogDisabled = true
		cfg.ApplicationLog = "/dev/null"
		cfg.Address = addr

		go func() {
			log.Fatal(skipper.Run(cfg.ToOptions()))
		}()

		initialized = true
		address = cfg.Address
	}

	conn, err := connect(address)

	if err != nil {
		log.Printf("failed to dial: %v\n", err)
		return -1
	}

	conn.Write(data)
	conn.Close()

	return 1
}
