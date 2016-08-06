package routing_test

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

type logSubscription struct {
	exp      string
	n        int
	response chan<- struct{}
}

type logWatch struct {
	entries []string
	reqs    []*logSubscription
}

type testLogger struct {
	save   chan string
	notify chan<- logSubscription
	clear  chan struct{}
	quit   chan<- struct{}
}

var errWaitTimeout = errors.New("timeout")

func (lw *logWatch) save(e string) {
	lw.entries = append(lw.entries, e)
	for i := len(lw.reqs) - 1; i >= 0; i-- {
		req := lw.reqs[i]
		if strings.Contains(e, req.exp) {
			req.n--
			if req.n <= 0 {
				close(req.response)
				lw.reqs = append(lw.reqs[:i], lw.reqs[i+1:]...)
			}
		}
	}
}

func (lw *logWatch) notify(req logSubscription) {
	for i := len(lw.entries) - 1; i >= 0; i-- {
		if strings.Contains(lw.entries[i], req.exp) {
			req.n--
			if req.n == 0 {
				break
			}
		}
	}

	if req.n <= 0 {
		close(req.response)
	} else {
		lw.reqs = append(lw.reqs, &req)
	}
}

func (lw *logWatch) clear() {
	lw.entries = nil
	lw.reqs = nil
}

func newTestLogger() *testLogger {
	lw := &logWatch{}
	save := make(chan string)
	notify := make(chan logSubscription)
	clear := make(chan struct{})
	quit := make(chan struct{})

	go func() {
		for {
			select {
			case e := <-save:
				lw.save(e)
			case req := <-notify:
				lw.notify(req)
			case <-clear:
				lw.clear()
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
	tl.notify <- logSubscription{exp, n, found}

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
