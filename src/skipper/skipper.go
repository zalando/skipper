package main

import "net/http"

func main() {
    http.ListenAndServe(":9090", &proxy{})
}
