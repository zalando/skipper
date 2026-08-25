package builtin

import (
	"time"

	"github.com/zalando/skipper/filters"
)

type timeoutType int

const (
	backendTimeout timeoutType = iota + 1
	readTimeout
	writeTimeout
)

type timeout struct {
	typ     timeoutType
	timeout time.Duration
}

func NewBackendTimeout() filters.Spec {
	return &timeout{
		typ: backendTimeout,
	}
}

func NewReadTimeout() filters.Spec {
	return &timeout{
		typ: readTimeout,
	}
}

func NewWriteTimeout() filters.Spec {
	return &timeout{
		typ: writeTimeout,
	}
}

func (t *timeout) Name() string {
	switch t.typ {
	case backendTimeout:
		return filters.BackendTimeoutName
	case readTimeout:
		return filters.ReadTimeoutName
	case writeTimeout:
		return filters.WriteTimeoutName
	}
	return "unknownFilter"
}

func (t *timeout) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	var tf timeout
	tf.typ = t.typ
	switch v := args[0].(type) {
	case string:
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, err
		}
		tf.timeout = d
	case time.Duration:
		tf.timeout = v
	default:
		return nil, filters.ErrInvalidFilterParameters
	}
	return &tf, nil
}

// Request allows overwrite of timeout settings.
//
// Type backend timeout sets the timeout for the backend roundtrip.
//
// Type read timeout sets the timeout to read the request including the body.
// It uses http.ResponseController to SetReadDeadline().
//
// Type write timeout allows to set a timeout for writing the response.
// It uses http.ResponseController to SetWriteDeadline().
//
// All these timeouts are set at specific points in proxy.Proxy.
func (t *timeout) Request(ctx filters.FilterContext) {
	switch t.typ {
	case backendTimeout:
		ctx.StateBag()[filters.BackendTimeout] = t.timeout
	case readTimeout:
		ctx.StateBag()[filters.ReadTimeout] = t.timeout
	case writeTimeout:
		ctx.StateBag()[filters.WriteTimeout] = t.timeout
	}
}

func (*timeout) Response(filters.FilterContext) {}
