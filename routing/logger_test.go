package routing_test

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

type testLogger struct {
	save   chan string
	notify chan<- struct {
		exp      string
		n        int
		response chan<- struct{}
	}
	clear chan struct{}
	quit  chan<- struct{}
}

var errWaitTimeout = errors.New("timeout")

func newTestLogger() *testLogger {
	save := make(chan string)
	notify := make(chan struct {
		exp      string
		n        int
		response chan<- struct{}
	})
	clear := make(chan struct{})
	quit := make(chan struct{})

	var (
		entries []string
		reqs    []*struct {
			exp      string
			n        int
			response chan<- struct{}
		}
	)

	go func() {
		for {
			select {
			case e := <-save:
				entries = append(entries, e)
				for i := len(reqs) - 1; i >= 0; i-- {
					req := reqs[i]
					if strings.Contains(e, req.exp) {
						req.n--
						if req.n <= 0 {
							close(req.response)
							reqs = append(reqs[:i], reqs[i+1:]...)
						}
					}
				}
			case req := <-notify:
				for i := len(entries) - 1; i >= 0; i-- {
					if strings.Contains(entries[i], req.exp) {
						req.n--
						if req.n == 0 {
							break
						}
					}
				}

				if req.n <= 0 {
					close(req.response)
				} else {
					reqs = append(reqs, &req)
				}
			case <-clear:
				entries = nil
				reqs = nil
			case <-quit:
				return
			}
		}
	}()

	return &testLogger{save, notify, clear, quit}
}

func (tl *testLogger) logf(f string, a ...interface{}) {
	log.Printf(f, a...)
	tl.save <- fmt.Sprintf(f, a...)
}

func (tl *testLogger) log(a ...interface{}) {
	log.Println(a...)
	tl.save <- fmt.Sprint(a...)
}

func (tl *testLogger) waitForN(exp string, n int, to time.Duration) error {
	found := make(chan struct{}, 1)
	tl.notify <- struct {
		exp      string
		n        int
		response chan<- struct{}
	}{exp, n, found}

	select {
	case <-found:
		return nil
	case <-time.After(to):
		return errWaitTimeout
	}
}

func (tl *testLogger) waitFor(exp string, to time.Duration) error {
	return tl.waitForN(exp, 1, to)
}

func (tl *testLogger) reset() {
	tl.clear <- struct{}{}
}

func (tl *testLogger) Close() {
	close(tl.quit)
}

func (tl *testLogger) Error(a ...interface{})            { tl.log(a...) }
func (tl *testLogger) Errorf(f string, a ...interface{}) { tl.logf(f, a...) }
func (tl *testLogger) Warn(a ...interface{})             { tl.log(a...) }
func (tl *testLogger) Warnf(f string, a ...interface{})  { tl.logf(f, a...) }
func (tl *testLogger) Info(a ...interface{})             { tl.log(a...) }
func (tl *testLogger) Infof(f string, a ...interface{})  { tl.logf(f, a...) }
func (tl *testLogger) Debug(a ...interface{})            { tl.log(a...) }
func (tl *testLogger) Debugf(f string, a ...interface{}) { tl.logf(f, a...) }
