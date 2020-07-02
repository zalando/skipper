package metrics

import (
	"fmt"
	"time"

	"github.com/zalando/skipper/filters"
)

type metricsType int

const (
	timer metricsType = iota
)

type filter struct {
	typ         metricsType
	name        string
	stateBagKey string
}

func NewTimer() filters.Spec {
	return metricsType(timer)
}

func (t metricsType) Name() string {
	switch metricsType(t) {
	case timer:
		return "timer"
	default:
		panic(fmt.Errorf("unsupported metrics filter type: %d", int(t)))
	}
}

func (t metricsType) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	name, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return filter{
		typ:         t,
		name:        name,
		stateBagKey: fmt.Sprintf("timer-filter-start-%s", name),
	}, nil
}

func (f filter) Request(ctx filters.FilterContext) {
	switch f.typ {
	case timer:
		ctx.StateBag()[f.stateBagKey] = time.Now()
	}
}

func (f filter) Response(ctx filters.FilterContext) {
	switch f.typ {
	case timer:
		start, ok := ctx.StateBag()[f.stateBagKey].(time.Time)
		if !ok {
			return
		}

		ctx.Metrics().MeasureSince(f.name, start)
	}
}
