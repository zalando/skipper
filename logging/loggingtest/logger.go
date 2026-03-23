// Package loggingtest is a test infrastructure package to support
// logging module tests.
package loggingtest

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

type logSubscription struct {
	exp    string
	n      int
	notify chan<- struct{}
}

type countMessage struct {
	expression string
	response   chan<- int
}

type logWatch struct {
	entries []string
	reqs    []*logSubscription
}

// Logger provides an implementation of the logging.Logger interface
// that can be used to receive notifications about log events.
type Logger struct {
	logc   chan string
	notify chan<- logSubscription
	count  chan countMessage
	clear  chan chan struct{}
	mute   chan bool
	quit   chan struct{}
}

// ErrWaitTimeout is returned when a logging event doesn't happen
// within a timeout.
var ErrWaitTimeout = errors.New("timeout")

func (lw *logWatch) save(e string) {
	lw.entries = append(lw.entries, e)
	for i := len(lw.reqs) - 1; i >= 0; i-- {
		req := lw.reqs[i]
		if strings.Contains(e, req.exp) {
			req.n--
			if req.n <= 0 {
				close(req.notify)
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
		close(req.notify)
	} else {
		lw.reqs = append(lw.reqs, &req)
	}
}

func (lw *logWatch) count(m countMessage) {
	var count int
	for _, e := range lw.entries {
		if strings.Contains(e, m.expression) {
			count++
		}
	}

	m.response <- count
}

func (lw *logWatch) clear() {
	lw.entries = nil
	lw.reqs = nil
}

// New returns a new, initialized instance of Logger.
func New() *Logger {
	lw := &logWatch{}
	logc := make(chan string)
	notify := make(chan logSubscription)
	count := make(chan countMessage)
	clear := make(chan chan struct{})
	mute := make(chan bool)
	quit := make(chan struct{})

	muted := !testing.Verbose()

	tl := &Logger{
		logc:   logc,
		notify: notify,
		count:  count,
		clear:  clear,
		mute:   mute,
		quit:   quit,
	}

	go func() {
		for {
			select {
			case e := <-logc:
				lw.save(e)
				if !muted {
					log.Println(e)
				}

			case req := <-notify:
				lw.notify(req)
			case req := <-count:
				lw.count(req)
			case c := <-clear:
				lw.clear()
				c <- struct{}{}
			case m := <-mute:
				muted = m
			case <-quit:
				return
			}
		}
	}()

	return tl
}

func (tl *Logger) logf(f string, a ...any) {
	select {
	case tl.logc <- fmt.Sprintf(f, a...):
	case <-tl.quit:
	}
}

func (tl *Logger) log(a ...any) {
	select {
	case tl.logc <- fmt.Sprint(a...):
	case <-tl.quit:
	}
}

// WaitForN returns nil when n logging events matching exp were received or returns
// ErrWaitTimeout when to timeout expired.
func (tl *Logger) WaitForN(exp string, n int, to time.Duration) error {
	found := make(chan struct{}, 1)
	tl.notify <- logSubscription{exp, n, found}

	select {
	case <-found:
		return nil
	case <-time.After(to):
		return ErrWaitTimeout
	}
}

// WaitFor returns nil when a logging event matching exp was received or returns
// ErrWaitTimeout when to timeout expired.
func (tl *Logger) WaitFor(exp string, to time.Duration) error {
	return tl.WaitForN(exp, 1, to)
}

// Count returns the recorded messages that match exp.
func (tl *Logger) Count(expression string) int {
	rsp := make(chan int)
	m := countMessage{
		expression: expression,
		response:   rsp,
	}

	tl.count <- m
	return <-rsp
}

// Reset clears the stored logging events.
func (tl *Logger) Reset() {
	ch := make(chan struct{})
	tl.clear <- ch
	<-ch
}

func (tl *Logger) Mute() {
	tl.mute <- true
}

func (tl *Logger) Unmute() {
	tl.mute <- false
}

// Close the logger.
func (tl *Logger) Close() {
	close(tl.quit)
}

func (tl *Logger) Error(a ...any)            { tl.log(a...) }
func (tl *Logger) Errorf(f string, a ...any) { tl.logf(f, a...) }
func (tl *Logger) Warn(a ...any)             { tl.log(a...) }
func (tl *Logger) Warnf(f string, a ...any)  { tl.logf(f, a...) }
func (tl *Logger) Info(a ...any)             { tl.log(a...) }
func (tl *Logger) Infof(f string, a ...any)  { tl.logf(f, a...) }
func (tl *Logger) Debug(a ...any)            { tl.log(a...) }
func (tl *Logger) Debugf(f string, a ...any) { tl.logf(f, a...) }

func (tl *Logger) Fire(entry *logrus.Entry) error {
	line, err := entry.String()
	if err != nil {
		return err
	}

	switch entry.Level {
	case logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel:
		tl.Error(line)
	case logrus.WarnLevel:
		tl.Warn(line)
	case logrus.InfoLevel:
		tl.Info(line)
	case logrus.DebugLevel, logrus.TraceLevel:
		tl.Debug(line)
	}

	return nil
}

func (tl *Logger) Levels() []logrus.Level {
	return logrus.AllLevels
}
