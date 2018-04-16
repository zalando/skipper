package loadbalancer

import (
	"fmt"

	"github.com/zalando/skipper/filters"
	"math/rand"
)

const (
	DecideFilterName = "lbDecide"
	DecisionHeader   = "X-Load-Balancer-Member"
)

type counter chan int

type decideSpec struct{}

type decideFilter struct {
	group   string
	size    int
	counter counter
}

func newCounter(size int) counter {
	c := make(counter, 1)
	c <- rand.Intn(size)
	return c
}

func (c counter) inc(groupSize int) int {
	v := <-c
	c <- v + 1
	return v % groupSize
}

func (c counter) value() int {
	v := <-c
	c <- v
	return v
}

// NewDecide create a filter specification for the decision route in
// load balancing scenarios. It expects two arguments: the name of the
// load balancing group, and the size of the load balancing group.
func NewDecide() filters.Spec { return &decideSpec{} }

func (s *decideSpec) Name() string { return DecideFilterName }

func (s *decideSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	group, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	size, ok := args[1].(int)
	if !ok {
		fsize, ok := args[1].(float64)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}

		size = int(fsize)
	}

	if size < 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &decideFilter{
		group:   group,
		size:    size,
		counter: newCounter(size),
	}, nil
}

func (f *decideFilter) Request(ctx filters.FilterContext) {
	current := f.counter.inc(f.size)
	ctx.Request().Header.Set(DecisionHeader, fmt.Sprintf("%s=%d", f.group, current))
}

func (f *decideFilter) Response(filters.FilterContext) {}
