package main

import "log"
import "net/http"

func main() {
    log.Fatal(http.ListenAndServe(":9090", &proxy{}))
}
