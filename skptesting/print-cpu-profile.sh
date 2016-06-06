#! /bin/bash

go tool pprof -text $GOPATH/bin/skptesting $GOPATH/src/github.com/zalando/skipper/skptesting/cpu-profile.prof 
