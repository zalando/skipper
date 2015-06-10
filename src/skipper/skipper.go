package main

import "log"
import "net/http"

func main() {
    ec := makeEtcdClient()
    ec.start()
    log.Fatal(http.ListenAndServe(":9090", &proxy{ec}))
}
