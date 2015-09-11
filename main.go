// skipper program main
//
// for a summary about skipper, please see the readme file.
package main

import (
	"flag"
	"github.com/zalando/skipper/run"
	"log"
	"strings"
	"time"
)

const (
	defaultAddress              = ":9090"
	defaultEtcdUrls             = "http://127.0.0.1:2379,http://127.0.0.1:4001"
	defaultStorageRoot          = "/skipper"
	defaultInnkeeperPollTimeout = int64(3 * time.Minute)

	addressUsage              = "address where skipper should listen on"
	etcdUrlsUsage             = "urls where etcd can be found"
	insecureUsage             = "set this flag to allow invalid certificates for tls connections"
	storageRootUsage          = "prefix for skipper related data in the provided etcd storage"
	innkeeperUrlUsage         = "url of the innkeeper API"
	innkeeperPollTimeoutUsage = "polling timeout of the innkeeper API"
	routesFileUsage           = "routes file to use instead of etcd"
)

var (
	address              string
	etcdUrls             string
	insecure             bool
	storageRoot          string
	innkeeperUrl         string
	innkeeperPollTimeout int64
	routesFile           string
)

func init() {
	flag.StringVar(&address, "address", defaultAddress, addressUsage)
	flag.StringVar(&etcdUrls, "etcd-urls", defaultEtcdUrls, etcdUrlsUsage)
	flag.BoolVar(&insecure, "insecure", false, insecureUsage)
	flag.StringVar(&storageRoot, "storage-root", defaultStorageRoot, storageRootUsage)
	flag.StringVar(&innkeeperUrl, "innkeeper-url", "https://innkeeper-tick-436125395.eu-west-1.elb.amazonaws.com/routes", innkeeperUrlUsage)
	flag.Int64Var(&innkeeperPollTimeout, "innkeeper-poll-timeout", defaultInnkeeperPollTimeout, innkeeperPollTimeoutUsage)
	flag.StringVar(&routesFile, "routes-file", "", routesFileUsage)
	flag.Parse()
}

func main() {
	log.Fatal(run.Run(run.Options{
		address,
		strings.Split(etcdUrls, ","),
		storageRoot,
		insecure,
		innkeeperUrl,
		time.Duration(innkeeperPollTimeout),
		routesFile,
		nil,
        false}))
}
