package proxylistener

import (
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/pires/go-proxyproto"
	snet "github.com/zalando/skipper/net"
)

const (
	defaultReadHeaderTimeout = time.Second // 10s seems too long https://github.com/pires/go-proxyproto/blob/5c8010d2392f09ce18169631c024aceae758335a/protocol.go#L28
	defaultReadBufferSize    = 256         // https://github.com/pires/go-proxyproto/blob/5c8010d2392f09ce18169631c024aceae758335a/protocol.go#L21
)

type Options struct {
	Listener          net.Listener
	ReadHeaderTimeout time.Duration
	ReadBufferSize    int

	AllowListCIDRs []string
	DenyListCIDRs  []string
	SkipListCIDRs  []string
}

func NewListener(opt Options) (net.Listener, error) {
	if opt.ReadHeaderTimeout == 0 {
		opt.ReadHeaderTimeout = defaultReadHeaderTimeout
	}
	if opt.ReadBufferSize == 0 {
		opt.ReadBufferSize = defaultReadBufferSize
	}

	skipSet, err := snet.ParseIPCIDRs(opt.SkipListCIDRs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse skip list: %w", err)
	}
	allowSet, err := snet.ParseIPCIDRs(opt.AllowListCIDRs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse allow list: %w", err)
	}
	denySet, err := snet.ParseIPCIDRs(opt.DenyListCIDRs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse deny list: %w", err)
	}

	policyLogic := func(cpo proxyproto.ConnPolicyOptions) (proxyproto.Policy, error) {
		host, _, err := net.SplitHostPort(cpo.Upstream.String())
		if err != nil {
			return proxyproto.REJECT, err
		}

		addr, err := netip.ParseAddr(host)
		if err != nil {
			return proxyproto.REJECT, err
		}

		if denySet.Contains(addr) {
			return proxyproto.REJECT, nil
		}
		if skipSet.Contains(addr) {
			return proxyproto.SKIP, nil
		}
		if allowSet.Contains(addr) {
			return proxyproto.USE, nil
		}

		return proxyproto.REJECT, nil
	}

	pl := &proxyproto.Listener{
		Listener:          opt.Listener,
		ReadHeaderTimeout: opt.ReadHeaderTimeout,
		ReadBufferSize:    opt.ReadBufferSize,

		ConnPolicy: policyLogic,
		ValidateHeader: func(h *proxyproto.Header) error {
			if h == nil {
				return fmt.Errorf("proxylistener: header is nil")
			}
			if h.SourceAddr == nil || h.DestinationAddr == nil {
				return fmt.Errorf("proxylistener: header missing addresses src: %q, dst: %q", h.SourceAddr, h.DestinationAddr)
			}
			if h.TransportProtocol != proxyproto.TCPv4 && h.TransportProtocol != proxyproto.TCPv6 {
				return fmt.Errorf("proxylistener: unsupported protocol %v", h.TransportProtocol)
			}
			return nil
		},
	}

	return pl, nil
}
