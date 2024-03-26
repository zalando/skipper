package net

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
)

type (
	ShutdownListener struct {
		net.Listener
		activeConns atomic.Int64
	}

	shutdownListenerConn struct {
		net.Conn
		listener *ShutdownListener
		once     sync.Once
	}
)

var _ net.Listener = &ShutdownListener{}

func NewShutdownListener(l net.Listener) *ShutdownListener {
	return &ShutdownListener{Listener: l}
}

func (l *ShutdownListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	l.registerConn()

	return &shutdownListenerConn{Conn: c, listener: l}, nil
}

func (l *ShutdownListener) Shutdown(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		n := l.activeConns.Load()
		log.Debugf("ShutdownListener Shutdown: %d active connections", n)
		if n == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *shutdownListenerConn) Close() error {
	err := c.Conn.Close()

	c.once.Do(c.listener.unregisterConn)

	return err
}

func (l *ShutdownListener) registerConn() {
	n := l.activeConns.Add(1)
	log.Debugf("ShutdownListener registerConn: %d active connections", n)
}

func (l *ShutdownListener) unregisterConn() {
	n := l.activeConns.Add(-1)
	log.Debugf("ShutdownListener unregisterConn: %d active connections", n)
}
