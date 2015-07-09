// composite filter that can be used as a base component for filters operating on request or response
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

   func (mw *impl) MakeFilter(id string, config skipper.FilterConfig) (skipper.Filter, error) {
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
	"github.com/zalando/skipper/filters/noop"
	"github.com/zalando/skipper/skipper"
)

const name = "_simpleheader"

// type implementing both skipper.FilterSpec and skipper.Filter
type Type struct {
	noop.Type
	id    string
	key   string
	value string
}

// the name of the filter spec
func (mw *Type) Name() string {
	return name
}

// call this method on the created filters to parse the filter config and to store the id
func (f *Type) InitFilter(id string, config skipper.FilterConfig) error {
	if len(config) != 2 {
		return errors.New("invalid number of args, expecting 2")
	}

	key, ok := config[0].(string)
	if !ok {
		return errors.New("invalid config type, expecting string")
	}

	value, ok := config[1].(string)
	if !ok {
		return errors.New("invalid config type, expecting string")
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
