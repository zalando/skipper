// composite middleware that can be used as a base component for middleware operating on request or response
// headers. The created filters are noop filters, unless the MakeFilter method is shadowed in the enclosing
// type. on the filter implementations, the InitFilter method can be used to parse the filter config, for
// the header key and value.
//
// Example:
/*
   type impl struct {
       simpleheader.Type
   }

   func (mw *impl) Name() string {
       return "header-filter"
   }

   func (mw *impl) MakeFilter(id string, config skipper.MiddlewareConfig) (skipper.Filter, error) {
       f := &impl{}
       err := f.InitFilter(id, config)
       if err != nil {
           return nil, err
       }

       return f, nil
   }

   func (f *impl) Request(ctx skipper.FilterContext) {
       ctx.Response().Header.Add(f.Key(), f.Value())
   }
*/
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

// type implementing both skipper.Middleware and skipper.Filter
type Type struct {
	noop.Type
	id    string
	key   string
	value string
}

// the name of the middeware
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

// call this method on the created filters to parse the filter config and to store the id
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

// the name (key) of the header
func (f *Type) Key() string {
	return f.key
}

// the value of the header (verbatim, no un-/escaping)
func (f *Type) Value() string {
	return f.value
}
