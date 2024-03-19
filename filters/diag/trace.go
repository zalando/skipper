package diag

import (
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

func getDurationArg(a interface{}) (time.Duration, error) {
	if s, ok := a.(string); ok {
		return time.ParseDuration(s)
	}
	return 0, filters.ErrInvalidFilterParameters
}

type traceSpec struct{}

type trace struct {
	d time.Duration
}

// NewTrace creates a filter specification for the trace() filter
func NewTrace() filters.Spec {
	return &traceSpec{}
}

func (*traceSpec) Name() string {
	return filters.TraceName
}

func (ts *traceSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	d, err := getDurationArg(args[0])
	if err != nil {
		log.Warnf("d failed on creation of trace(): %v", err)
		return nil, filters.ErrInvalidFilterParameters
	}

	return &trace{
		d: d,
	}, nil
}

func (tr *trace) Request(ctx filters.FilterContext) {
	ctx.StateBag()[filters.TraceName] = tr.d
}

func (*trace) Response(filters.FilterContext) {}
