package main

import "log"
import "net/http"
import "skipper/etcd"
import "skipper/proxy"
import "skipper/settingssource"

func main() {
	e := etcd.TempMock()
	s := settingssource.MakeSource(e)
	p := proxy.Make(s)
	log.Fatal(http.ListenAndServe(":9090", p))
}
