package loadbalancer

import (
	"math/rand"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

type lbAlgorithmType int

const (
	RoundRobin         = "roundRobin"
	unknownLbAlgorithm = "unknownLbAlgorithm"

	roundrobin lbAlgorithmType = iota
)

type lbAlgorithmSpec struct {
	typ lbAlgorithmType
}

// NewRoundRobin returns a roundrobin filter spec
func NewRoundRobin() filters.Spec { return &lbAlgorithmSpec{typ: roundrobin} }

func (spec *lbAlgorithmSpec) Name() string {
	switch spec.typ {
	case roundrobin:
		return RoundRobin
	default:
		return unknownLbAlgorithm
	}
}

func (spec *lbAlgorithmSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	switch spec.typ {
	case roundrobin:
		return &roundrobinLB{
			idx: -1,
		}, nil
	default:
		return nil, filters.ErrInvalidFilterParameters
	}
}

type roundrobinLB struct {
	mu  sync.Mutex
	idx int
}

// Request use endpoints list from statebag and applies the configured
// algorithm to set the backend.
func (lba *roundrobinLB) Request(ctx filters.FilterContext) {
	endpoints, ok := ctx.StateBag()[statebagEndpointsKey].([]string)
	if !ok {
		logrus.Error("Failed to get endpoints from statebag")
		return
	}
	ep := lba.apply(endpoints)
	ctx.StateBag()[filters.DynamicBackendURLKey] = ep
}

// Response has nothing to do
func (*roundrobinLB) Response(filters.FilterContext) {}

// apply the roundrobin loadbalancer algorithm to the given
// endpoints to select the current target
func (lba *roundrobinLB) apply(endpoints []string) string {
	lba.mu.Lock()
	defer lba.mu.Unlock()
	if lba.idx < 0 {
		lba.idx = rand.Intn(len(endpoints))
		return endpoints[lba.idx]
	}
	lba.idx = (lba.idx + 1) % len(endpoints)
	return endpoints[lba.idx]
}
