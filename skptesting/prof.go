package main

import (
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"github.com/zalando/skipper"
)

func main() {
	profile := "profile.prof"
	if len(os.Args) > 1 {
		profile = os.Args[1]
	}

	address := ":9090"
	if len(os.Args) > 2 {
		address = os.Args[2]
	}

	routesFile := "routes.eskip"
	if len(os.Args) > 3 {
		routesFile = os.Args[3]
	}

	p, err := os.Create(profile)
	if err != nil {
		log.Fatal(err)
	}

	pprof.StartCPUProfile(p)
	defer func() {
		pprof.StopCPUProfile()
		println("profile flushed")
	}()

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)

	go func() {
		err = skipper.Run(skipper.Options{
			AccessLogDisabled: true,
			Address:           address,
			RoutesFile:        routesFile})
		if err != nil {
			log.Fatal(err)
		}
	}()

	<-sigint
}
