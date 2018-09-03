package monitoring

import (
	"fmt"
	"github.com/zalando/skipper/filters"
	"strconv"
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

	// arg 0 "bar": a mandatory string
	if l < 1 {
		log.Error("Calling filter with no argument")
		return nil, filters.ErrInvalidFilterParameters
	}
	arg0 := args[0]
	bar, ok := arg0.(string)
	if !ok {
		log.Errorf("Calling filter with arg[0] not a string: %+v", arg0)
		return nil, filters.ErrInvalidFilterParameters
	}

	// arg 1 "num": an optional uint64
	var num *uint64 = nil
	if l > 1 {
		arg1 := fmt.Sprintf("%v", args[1])
		arg1Uint64, err := strconv.ParseUint(arg1, 10, 64)
		if err != nil {
			log.Errorf("Calling filter with arg[1] not a uint64: %+v", arg1)
			return nil, filters.ErrInvalidFilterParameters
		}
		num = &arg1Uint64
	}

	filter = &monitoringFilter{
		bar: bar,
		num: num,
	}
	log.Infof("Created filter: %+v", filter)
	return
}
