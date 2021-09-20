package routesrv_test

import (
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routesrv"
)

var tl *loggingtest.Logger

func TestMain(m *testing.M) {
	flag.Parse()
	tl = loggingtest.New()
	logrus.AddHook(tl)
	os.Exit(m.Run())
}

func TestDummy(t *testing.T) {
	go func() {
		routesrv.Run(routesrv.Options{
			SourcePollTimeout: 3 * time.Second,
		})
	}()

	if err := tl.WaitFor(routesrv.LogPollingStarted, 2*time.Microsecond); err != nil {
		t.Error("polling did not start")
	}

	fmt.Println("after here")
}
