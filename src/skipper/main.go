package main

import "log"
import "net/http"
import "skipper/etcd"
import "skipper/proxy"
import "skipper/settings"

func main() {
	e := etcd.TempMock()
	ss := settings.MakeSource(e, nil)
	p := proxy.Make(ss)
	s := <-ss.Get()
	log.Fatal(http.ListenAndServe(s.Address(), p))
}
