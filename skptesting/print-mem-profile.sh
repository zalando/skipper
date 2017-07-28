#! /bin/bash

go tool pprof -text $GOPATH/src/github.com/zalando/skipper/skptesting/mem-profile.prof 
