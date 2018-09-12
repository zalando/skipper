package monitoring

import (
	"fmt"
	"github.com/zalando/skipper/filters"
)

type monitoringSpec struct {
	Foo string
}

var _ filters.Spec = &monitoringSpec{}

func (s *monitoringSpec) Name() string {
	return name
}

func (s *monitoringSpec) CreateFilter(args []interface{}) (filter filters.Filter, err error) {
	defer func() {
		if r := recover(); r != nil {
			switch x := r.(type) {
			case error:
				err = x
			default:
				err = fmt.Errorf("%v", x)
			}
		}
	}()

	log.Info("Create new filter")

	l := len(args)
	log.Infof("DEBUG: l = %#v", l)
	// arg 0 "apiId": an optional string with an API identifier.
	apiId := ""
	if l > 0 {
		arg0 := args[0]
		log.Infof("DEBUG: arg0 = %#v", arg0)
		arg0s, ok := arg0.(string)
		log.Infof("DEBUG: arg0s = %#v / ok = %#v", arg0s, ok)
		if !ok {
			log.Errorf("Calling filter with arg[0] (apiId) not a string: %+v", arg0)
			return nil, filters.ErrInvalidFilterParameters
		}
		apiId = arg0s
	}

	// Create the filter
	filter = &monitoringFilter{
		apiId: apiId,
	}
	log.Infof("Created filter: %+v", filter)
	return
}
