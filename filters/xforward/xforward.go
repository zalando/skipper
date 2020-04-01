package xforward

import (
	"fmt"
	"net"

	"github.com/zalando/skipper/filters"
)

const (
	// Name of the "xforward" filter.
	Name = "xforward"

	// NameFirst is the name of the "xforwardFirst" filter.
	NameFirst = "xforwardFirst"
)

type filter struct {
	prepend bool
}

// New creates a specification for the xforward filter
// that appends the remote IP of the incoming request to the
// X-Forwarded-For header, and sets the X-Forwarded-Host header
// to the value of the incoming request's Host header.
func New() filters.Spec {
	return filter{}
}

// NewFirst creates a specification for the xforwardFirst filter
// that prepends the remote IP of the incoming request to the
// X-Forwarded-For header, and sets the X-Forwarded-Host header
// to the value of the incoming request's Host header.
func NewFirst() filters.Spec {
	return filter{prepend: true}
}

func (f filter) Name() string {
	if f.prepend {
		return NameFirst
	}

	return Name
}

func (f filter) CreateFilter([]interface{}) (filters.Filter, error) {
	return filter(f), nil
}

func (f filter) Request(ctx filters.FilterContext) {
	req := ctx.OriginalRequest()
	if req == nil {
		req = ctx.Request()
	}

	req.Header.Set("X-Forwarded-Host", req.Host)
	if req.RemoteAddr == "" {
		return
	}

	addr := req.RemoteAddr
	if h, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		addr = h
	}

	h := req.Header.Get("X-Forwarded-For")
	if h == "" {
		h = addr
	} else if f.prepend {
		h = fmt.Sprintf("%s, %s", addr, h)
	} else {
		h = fmt.Sprintf("%s, %s", h, addr)
	}

	req.Header.Set("X-Forwarded-For", h)
}

func (filter) Response(filters.FilterContext) {}
