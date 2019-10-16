package queuelistener

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/aryszka/jobqueue"
	log "github.com/sirupsen/logrus"
)

// these values may need adjustments for the experiment
const (
	defaultMaxConcurrency = 3000  // 6000
	defaultMaxQueueSize   = 30000 // 3000
)

// implements net.Error to support Temporary()
type queueError struct {
	err error
}

// implements net.Conn to support jobqueue's done() callback on Close
//
// !!! Problem: Calling done() only on Close() may not be enough, because
// that would assume that Close() is always called. This has to be
// verified for 100% because otherwise we may block the server infinitely
// on occasions.
type connection struct {
	net  net.Conn
	done func()
}

// implements net.Listener to queue the incoming connections
type listener struct {
	net net.Listener
	q   *jobqueue.Stack
}

func combineErrors(errs ...error) error {
	if len(errs) == 0 {
		return errors.New("unknown error(s)")
	}

	s := make([]string, len(errs))
	for i := range errs {
		s[i] = errs[i].Error()
	}

	return fmt.Errorf("multiple errors: %v", strings.Join(s, "; "))
}

func (ne *queueError) Error() string   { return fmt.Sprintf("listener queue error: %v", ne.err) }
func (ne *queueError) Timeout() bool   { return ne.err == jobqueue.ErrTimeout }
func (ne *queueError) Temporary() bool { return ne.err != jobqueue.ErrClosed }

func (c *connection) Read(b []byte) (n int, err error)   { return c.net.Read(b) }
func (c *connection) Write(b []byte) (n int, err error)  { return c.net.Write(b) }
func (c *connection) LocalAddr() net.Addr                { return c.net.LocalAddr() }
func (c *connection) RemoteAddr() net.Addr               { return c.net.RemoteAddr() }
func (c *connection) SetDeadline(t time.Time) error      { return c.net.SetDeadline(t) }
func (c *connection) SetReadDeadline(t time.Time) error  { return c.net.SetReadDeadline(t) }
func (c *connection) SetWriteDeadline(t time.Time) error { return c.net.SetWriteDeadline(t) }

func (c *connection) Close() error {
	defer func() {
		c.done()
		c.done = func() {}
	}()

	return c.net.Close()
}

func Listen(network, address string) (net.Listener, error) {
	l, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}

	var (
		maxConcurrency = defaultMaxConcurrency
		maxQueueSize   = defaultMaxQueueSize
		timeout        = time.Minute
	)

	{
		memoryLimitFile := "/sys/fs/cgroup/memory/memory.limit_in_bytes"
		memoryLimitBytes, err := ioutil.ReadFile(memoryLimitFile)
		if err != nil {
			log.Errorf("Failed to read memory limits, fallback to defaults: %v", err)
		} else {
			memoryLimitString := string(memoryLimitBytes[0 : len(memoryLimitBytes)-2])
			println("memoryLimitString:", memoryLimitString)
			memoryLimit, err := strconv.Atoi(memoryLimitString)
			if err != nil {
				log.Errorf("Failed to convert memory limits, fallback to defaults: %v", err)
			} else {
				memLimit := memoryLimit

				factor := int(float64(memLimit) / 1024.0 / 1000.0 / 100.0) // 100MB
				if factor < 1 {
					factor = 1
				}
				maxConcurrency = 1000 * factor
				maxQueueSize = 10 * maxConcurrency

			}
		}
	}

	log.Infof("TCP listener with LIFO queue settings: MaxConcurrency=%d MaxStackSize=%d Timeout=%s", maxConcurrency, maxQueueSize, timeout)

	return &listener{
		net: l,
		q: jobqueue.With(jobqueue.Options{
			MaxConcurrency: maxConcurrency,
			MaxStackSize:   maxQueueSize,
			Timeout:        timeout,
			CloseTimeout:   time.Second,
		}),
	}, nil
}

func (l *listener) Accept() (net.Conn, error) {
	c, err := l.net.Accept()
	if err != nil {
		return nil, err
	}

	done, err := l.q.Wait()
	if err != nil && err == jobqueue.ErrClosed {
		var qerr error = &queueError{err: err}
		if cerr := c.Close(); cerr != nil {
			qerr = combineErrors(qerr, cerr)
		}

		return nil, qerr
	}

	if err != nil {
		return nil, &queueError{err: err}
	}

	return &connection{net: c, done: done}, nil
}

func (l *listener) Close() error {
	nerr := l.net.Close()
	qerr := l.q.Close()
	if nerr != nil && qerr != nil {
		return combineErrors(nerr, qerr)
	}

	if nerr != nil {
		return nerr
	}

	return qerr
}

func (l *listener) Addr() net.Addr {
	return l.net.Addr()
}
