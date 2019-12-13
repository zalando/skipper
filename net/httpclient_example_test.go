package net_test

import (
	"log"
	"net/http"

	"github.com/lightstep/lightstep-tracer-go"
	"github.com/zalando/skipper/net"
)

func ExampleTransport() {
	quit := make(chan struct{})
	tracer := lightstep.NewTracer(lightstep.Options{})

	cli := net.NewTransport(net.Options{
		Tracer: tracer,
	}, quit)
	cli = net.WithSpanName(cli, "myspan")
	cli = net.WithBearerToken(cli, "mytoken")

	u := "http://127.0.0.1:12345/foo"
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}

	rsp, err := cli.RoundTrip(req)
	if err != nil {
		log.Fatalf("Failed to do request: %v", err)
	}
	log.Printf("rsp code: %v", rsp.StatusCode)
}
