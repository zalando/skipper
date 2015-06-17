package requestheader

import (
	"errors"
	"net/http"
	"skipper/middleware/noop"
	"skipper/skipper"
)

const (
	name            = "request-header"
	keyConfigName   = "key"
	valueConfigName = "value"
)

type impl struct {
	noop.Type
	id    string
	key   string
	value string
}

func Make() skipper.Middleware {
	return &impl{}
}

func (mw *impl) Name() string {
	return name
}

func stringValue(config skipper.MiddlewareConfig, name string) (string, bool) {
	if ival, ok := config[name]; ok {
		if val, ok := ival.(string); ok {
			return val, true
		}
	}

	return "", false
}

func (mw *impl) MakeFilter(id string, config skipper.MiddlewareConfig) (skipper.Filter, error) {
	var (
		key, value string
		ok         bool
	)

	if key, ok = stringValue(config, keyConfigName); !ok {
		return nil, errors.New("missing key")
	}

	if value, ok = stringValue(config, valueConfigName); !ok {
		return nil, errors.New("missing value")
	}

	f := &impl{key: key, value: value}
	f.SetId(id)
	return f, nil
}

func (f *impl) ProcessRequest(r *http.Request) *http.Request {
	r.Header.Add(f.key, f.value)
	return r
}
