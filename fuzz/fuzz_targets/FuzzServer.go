//go:build gofuzz
// +build gofuzz

package fuzz

import (
	"errors"
	"log"
	"net"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper"
	"github.com/zalando/skipper/config"
)

var (
	address     = ""
	initialized = false
)

func find_address() (string, error) {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})

	if err != nil {
		return "", err
	}

	defer l.Close()

	return l.Addr().String(), nil
}

func connect(host string) (net.Conn, error) {
	for i := 0; i < 15; i++ {
		conn, err := net.Dial("tcp", host)

		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		return conn, err
	}

	return nil, errors.New("unable to connect")
}

func run_server() {
	addr, err := find_address()

	if err != nil {
		log.Printf("failed to find address: %v\n", err)
		os.Exit(-1)
	}

	cfg := config.NewConfig()
	cfg.InlineRoutes = `r: * -> status(200) -> inlineContent("ok") -> <shunt>`
	if l, err := logrus.ParseLevel("panic"); err == nil {
		cfg.ApplicationLogLevel = l
	}
	cfg.AccessLogDisabled = true
	cfg.ApplicationLog = "/dev/null"
	cfg.Address = addr
	cfg.SupportListener = "127.0.0.1:0"

	go func() {
		log.Fatal(skipper.Run(cfg.ToOptions()))
	}()

	address = cfg.Address
}

func FuzzServer(data []byte) int {

	if !initialized {
		run_server()
		initialized = true
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
