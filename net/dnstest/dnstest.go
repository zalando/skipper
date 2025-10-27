// Package dnstest is a test infrastructure package to be able to
// control DNS resolution.
package dnstest

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// LoopbackNames replaces net.DefaultResolver with a resolver that resolves
// configured names to 127.0.0.1 and fails to resolve any other name.
// Uses t.Cleanup to restore resolver after the test.
func LoopbackNames(t *testing.T, first string, rest ...string) {
	mux := dns.NewServeMux()
	mux.HandleFunc(".", newDnsHandler(append(rest, first)))

	s, err := startServer(mux)
	if err != nil {
		t.Fatal(err)
		return
	}
	defaultResolver := net.DefaultResolver
	net.DefaultResolver = serverResolver(s)

	t.Cleanup(func() {
		net.DefaultResolver = defaultResolver
		s.Shutdown()
	})
}

type dnsHandler struct {
	names map[string]struct{}
}

func newDnsHandler(names []string) func(w dns.ResponseWriter, r *dns.Msg) {
	h := &dnsHandler{
		names: make(map[string]struct{}),
	}
	for _, name := range names {
		h.names[dns.CanonicalName(name)] = struct{}{}
	}
	return h.handle
}

func (h *dnsHandler) handle(w dns.ResponseWriter, r *dns.Msg) {
	if r.MsgHdr.Opcode != dns.OpcodeQuery {
		h.nameError(w, r)
		return
	}
	q := r.Question[0]
	if q.Qtype != dns.TypeA || q.Qclass != dns.ClassINET {
		h.nameError(w, r)
		return
	}

	qname := dns.CanonicalName(q.Name)
	if _, ok := h.names[qname]; !ok {
		h.nameError(w, r)
		return
	}

	reply := new(dns.Msg)
	reply.SetRcode(r, dns.RcodeSuccess)
	reply.Answer = append(reply.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   qname,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    9999,
		},
		A: net.IPv4(127, 0, 0, 1),
	})
	w.WriteMsg(reply)
}

func (h *dnsHandler) nameError(w dns.ResponseWriter, r *dns.Msg) {
	reply := new(dns.Msg)
	reply.SetRcode(r, dns.RcodeNameError)
	w.WriteMsg(reply)
}

func startServer(handler dns.Handler) (*dns.Server, error) {
	ready := make(chan error, 1)
	server := &dns.Server{
		Addr:              ":0",
		Net:               "udp",
		Handler:           handler,
		NotifyStartedFunc: func() { ready <- nil },
	}
	go func() { ready <- server.ListenAndServe() }()
	return server, <-ready
}

func serverResolver(server *dns.Server) *net.Resolver {
	return &net.Resolver{
		PreferGo:     true,
		StrictErrors: true,
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if network != "udp" {
				return nil, fmt.Errorf("unsupported network: %s", network)
			}
			dialer := net.Dialer{Timeout: 1 * time.Second}
			return dialer.DialContext(ctx, network, server.PacketConn.LocalAddr().String())
		},
	}
}
