package simpleheader

import (
	"errors"
	"skipper/middleware/noop"
	"skipper/skipper"
)

const (
	name            = "_simple-header"
	keyConfigName   = "key"
	valueConfigName = "value"
)

type Type struct {
	noop.Type
	id    string
	key   string
	value string
}

func (mw *Type) Name() string {
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

func (f *Type) InitFilter(id string, config skipper.MiddlewareConfig) error {
	var (
		key, value string
		ok         bool
	)

	if key, ok = stringValue(config, keyConfigName); !ok {
		return errors.New("missing key")
	}

	if value, ok = stringValue(config, valueConfigName); !ok {
		return errors.New("missing value")
	}

	f.key = key
	f.value = value
	f.SetId(id)
	return nil
}

func (f *Type) Key() string {
	return f.key
}

func (f *Type) Value() string {
	return f.value
}
