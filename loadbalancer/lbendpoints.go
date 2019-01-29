package loadbalancer

import "github.com/zalando/skipper/filters"

const (
	LBEndpointsName      = "lbEndpoints"
	statebagEndpointsKey = "endpoints"
)

type lbEndpointsSpec struct{}

// NewLBEndpoints create a filter lbEndpointsSpecification for the lbEndpoints filter.
func NewLBEndpoints() filters.Spec { return &lbEndpointsSpec{} }

func (s *lbEndpointsSpec) Name() string {
	return LBEndpointsName
}

func (s *lbEndpointsSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	endpoints := make([]string, len(args))

	for i, argi := range args {
		if s, ok := argi.(string); ok {
			endpoints[i] = s
		} else {
			return nil, filters.ErrInvalidFilterParameters
		}
	}
	return &lbEndpointsFilter{endpoints: endpoints}, nil
}

type lbEndpointsFilter struct {
	endpoints []string
}

// Request is binding the configured endpoints list to statebag
// entries which will be handled by the lbAlgorithm filter.
func (f *lbEndpointsFilter) Request(ctx filters.FilterContext) {
	ctx.StateBag()[statebagEndpointsKey] = f.endpoints
}

// Response has nothing to do
func (*lbEndpointsFilter) Response(filters.FilterContext) {}
