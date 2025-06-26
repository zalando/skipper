package net

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/zalando/skipper/metrics"
)

// ConnManager tracks creation of HTTP server connections and
// closes connections when their age or number of requests served reaches configured limits.
// Use [ConnManager.Configure] method to setup ConnManager for an [http.Server].
type ConnManager struct {
	// Metrics is an optional metrics registry to count connection events.
	Metrics metrics.Metrics

	// Keepalive is the duration after which server connection is closed.
	Keepalive time.Duration

	// KeepaliveRequests is the number of requests after which server connection is closed.
	KeepaliveRequests int

	handler http.Handler
}

type connState struct {
	expiresAt time.Time
	requests  int
}

type contextKey struct{}

var connection contextKey

func (cm *ConnManager) Configure(server *http.Server) {
	cm.handler = server.Handler
	server.Handler = http.HandlerFunc(cm.serveHTTP)

	if cc := server.ConnContext; cc != nil {
		server.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
			ctx = cc(ctx, c)
			return cm.connContext(ctx, c)
		}
	} else {
		server.ConnContext = cm.connContext
	}

	if cs := server.ConnState; cs != nil {
		server.ConnState = func(c net.Conn, state http.ConnState) {
			cs(c, state)
			cm.connState(c, state)
		}
	} else {
		server.ConnState = cm.connState
	}
}

func (cm *ConnManager) serveHTTP(w http.ResponseWriter, r *http.Request) {
	state, ok := r.Context().Value(connection).(*connState)
	if !ok || state == nil {
		cm.handler.ServeHTTP(w, r)
		return
	}
	state.requests++

	if cm.KeepaliveRequests > 0 && state.requests >= cm.KeepaliveRequests {
		w.Header().Set("Connection", "close")

		cm.count("lb-conn-closed.keepalive-requests")
	}

	if cm.Keepalive > 0 && time.Now().After(state.expiresAt) {
		w.Header().Set("Connection", "close")

		cm.count("lb-conn-closed.keepalive")
	}

	cm.handler.ServeHTTP(w, r)
}

func (cm *ConnManager) connContext(ctx context.Context, _ net.Conn) context.Context {
	state := &connState{
		expiresAt: time.Now().Add(cm.Keepalive),
	}
	return context.WithValue(ctx, connection, state)
}

func (cm *ConnManager) connState(_ net.Conn, state http.ConnState) {
	cm.count(fmt.Sprintf("lb-conn-%s", state))
}

func (cm *ConnManager) count(name string) {
	if cm.Metrics != nil {
		cm.Metrics.IncCounter(name)
	}
}
