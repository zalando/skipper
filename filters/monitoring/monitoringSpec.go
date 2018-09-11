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
	filter = &monitoringFilter{}
	log.Infof("Created filter: %+v", filter)
	return
}
