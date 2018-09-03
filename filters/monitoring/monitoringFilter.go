package monitoring

import (
	"fmt"
	"github.com/zalando/skipper/filters"
	"io"
	"time"
)

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
