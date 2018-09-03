package monitoring

import (
	"github.com/zalando/skipper/filters"
	"github.com/sirupsen/logrus"
	"io"
	"fmt"
	"strconv"
	"time"
)

const (
	name = "monitor"
)

var (
	log = logrus.WithField("filter", "monitoring")
)

func NewMonitoring(foo string) filters.Spec {
	log.Infof("Create new filter spec with `foo` %q", foo)
	return &monitoringSpec{
		Foo: foo,
	}
}

// ==============================================================

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

// ==============================================================

type monitoringFilter struct {
	bar string
	num *uint64
	begin time.Time
}

var _ filters.Filter = &monitoringFilter{}
var _ io.Closer = &monitoringFilter{}
var _ fmt.Stringer = &monitoringFilter{}
var _ fmt.GoStringer = &monitoringFilter{}

func (f *monitoringFilter) Request(_ filters.FilterContext) {
	f.begin = time.Now()
	log.Infof("Request! %+v", f)
}

func (f *monitoringFilter) Response(_ filters.FilterContext) {
	log.Infof("Response! %+v", f)

	end := time.Now()
	dur := end.Sub(f.begin)
	log.WithField("track", "call_duration").Infof("Call took %v", dur)
}

func (f *monitoringFilter) Close() error {
	log.Infof("Close! %+v", f)
	return nil
}

func (f monitoringFilter) String() string {
	return f.GoString()
}

func (f monitoringFilter) GoString() string {
	var numPart string
	if f.num == nil {
		numPart = "nil"
	} else {
		numPart = fmt.Sprintf("%v", *f.num)
	}
	return fmt.Sprintf("%T{bar:%q, num:%s}", f, f.bar, numPart)
}
