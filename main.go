// skipper program main
//
// command line flags:
//
// -etcd-urls:
// one or more urls where the etcd service to be used can be found
//
// -insecure:
// if set, skipper accepts invalid certificates from backend hosts
//
// (see the skipper package for an overview of the program structure)
package main

import (
	"flag"
	"github.com/zalando/skipper/run"
	"log"
	"strings"
)

const (
	defaultAddress  = ":9090"
	defaultEtcdUrls = "http://127.0.0.1:2379,http://127.0.0.1:4001"
	addressUsage    = "address where skipper should listen on"
	etcdUrlsUsage   = "urls where etcd can be found"
	insecureUsage   = "set this flag to allow invalid certificates for tls connections"
)

var (
	address  string
	etcdUrls string
	insecure bool
)

func init() {
	flag.StringVar(&address, "address", defaultAddress, addressUsage)
	flag.StringVar(&etcdUrls, "etcd-urls", defaultEtcdUrls, etcdUrlsUsage)
	flag.BoolVar(&insecure, "insecure", false, insecureUsage)
	flag.Parse()
}

func main() {
	log.Fatal(run.Run(address, strings.Split(etcdUrls, ","), insecure))
}
