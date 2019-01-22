// +build ignore

package main

import (
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

func main() {
	tr := &http.Transport{
		TLSHandshakeTimeout:   1 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 5 * time.Second,
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       8 * time.Second,
	}

	quit := make(chan struct{})
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	target := "http://127.0.0.1:9090"
	request, err := http.NewRequest("GET", target, nil)
	if err != nil {
		logrus.Fatalf("Failed to request: %v", err)
	}
	request.Host = "test.example.org"

	go func(req *http.Request, q chan struct{}) {
		for {
			select {
			case <-time.After(100 * time.Millisecond):
				resp, err2 := tr.RoundTrip(req)
				if err2 != nil {
					logrus.Errorf("Failed to roundtrip: %v", err2)
				}
				defer resp.Body.Close()

				body, err2 := ioutil.ReadAll(resp.Body)
				if err2 != nil {
					logrus.Errorf("Failed to read body: %v", err2)
				}
				logrus.Infof("resp body: %s", body)
			case <-q:
				return
			}
		}
	}(request, quit)

	time.Sleep(3 * time.Second)

	ureq, _ := http.NewRequest("GET", target, nil)
	ureq.Host = "wrong.example.org"
	ureq.Header.Set("Upgrade", "websocket")
	ureq.Header.Set("Connection", "upgrade")
	uresp, err := tr.RoundTrip(ureq)
	if err != nil {
		logrus.Errorf("Failed to do ugrade roundtrip: %v", err)
	} else {

		defer uresp.Body.Close()
		b, err := ioutil.ReadAll(uresp.Body)
		if err != nil {
			logrus.Errorf("Failed to read upgrade body: %v", err)
		}
		logrus.Infof("uresp: %s", b)
	}
	<-c
	quit <- struct{}{}
}
