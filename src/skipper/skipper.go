package main

import "log"
import "net/http"
import "skipper/etcd"
import "skipper/proxy"

func main() {
	ec := etcd.MakeEtcdClient()
	ec.Start()
	log.Fatal(http.ListenAndServe(":9090", proxy.MakeProxy(ec)))
}
