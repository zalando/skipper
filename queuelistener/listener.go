package queuelistener

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/aryszka/jobqueue"
	log "github.com/sirupsen/logrus"
)

const (
	defaultMaxConcurrency = 3000
	defaultMaxQueueSize   = 30000
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

func Listen(memoryLimit, bytesPerRequest int, network, address string) (net.Listener, error) {
	l, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}

	var (
		maxConcurrency = defaultMaxConcurrency
		maxQueueSize   = defaultMaxQueueSize
		timeout        = time.Minute
	)

	if memoryLimit > 0 && memoryLimit >= bytesPerRequest {
		maxConcurrency = memoryLimit / bytesPerRequest
		maxQueueSize = 10 * maxConcurrency
	}

	log.Infof("TCP listener with LIFO queue settings: MaxConcurrency=%d MaxStackSize=%d Timeout=%s", maxConcurrency, maxQueueSize, timeout)

	return &listener{
		net: l,
		q: jobqueue.With(jobqueue.Options{
			MaxConcurrency: maxConcurrency,
			MaxStackSize:   maxQueueSize,
			Timeout:        timeout,
			CloseTimeout:   100 * time.Second,
		}),
	}, nil
}

func (l *listener) Accept() (net.Conn, error) {
	var (
		c   net.Conn
		err error
		ok  bool
	)
	fmt.Println(l.q.Status())
	//c, ok = l.q.Dequeue()
	if !ok {
		c, err = l.net.Accept()
		if err != nil {
			return nil, err
		}
	}

	println("waiting")
	done, err := l.q.Wait()
	if err != nil && err == jobqueue.ErrClosed {
		println("queue closed")
		var qerr error = &queueError{err: err}
		if cerr := c.Close(); cerr != nil {
			qerr = combineErrors(qerr, cerr)
		}

		return nil, qerr
	}

	if err != nil {
		println("queueError")
		return nil, &queueError{err: err}
	}

	println("success")
	return &connection{net: c, done: done}, nil
}

func (l *listener) Close() error {
	fmt.Println(l.q.Status())
	l.q.Close()
	return l.net.Close()
}

func (l *listener) Addr() net.Addr {
	return l.net.Addr()
}
