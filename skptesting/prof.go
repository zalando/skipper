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
	cpuProfile := "cpu-profile.prof"
	if len(os.Args) > 1 {
		cpuProfile = os.Args[1]
	}

	memProfile := "mem-profile.prof"
	if len(os.Args) > 2 {
		memProfile = os.Args[2]
	}

	address := ":9090"
	if len(os.Args) > 3 {
		address = os.Args[3]
	}

	routesFile := "routes.eskip"
	if len(os.Args) > 4 {
		routesFile = os.Args[4]
	}

	cpuOut, err := os.Create(cpuProfile)
	if err != nil {
		log.Fatal(err)
	}

	memOut, err := os.Create(memProfile)
	if err != nil {
		log.Fatal(err)
	}

	pprof.StartCPUProfile(cpuOut)
	defer func() {
		pprof.StopCPUProfile()
		pprof.Lookup("heap").WriteTo(memOut, 0)
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
