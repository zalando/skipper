package traffic

import (
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

type (
	segmentSpec struct {
		randFloat64 func() float64
	}
	segmentPredicate struct {
		randFloat64 func() float64
		min, max    float64
	}
)

type contextKey struct{}

var randomValue contextKey

// NewSegment creates a new traffic segment predicate specification
func NewSegment() routing.WeightedPredicateSpec {
	src := &lockedSource{s: rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano()/2))}
	return &segmentSpec{rand.New(src).Float64}
}

func (*segmentSpec) Name() string {
	return predicates.TrafficSegmentName
}

// Create new predicate instance with two number arguments _min_ and _max_
// from an interval [0, 1] (from zero included to one included) and _min_ <= _max_.
//
// Let _r_ be one-per-request uniform random number value from [0, 1).
// This predicate matches if _r_ belongs to an interval from [_min_, _max_).
// Upper interval boundary _max_ is excluded to simplify definition of
// adjacent intervals - the upper boundary of the first interval
// then equals lower boundary of the next and so on, e.g. [0, 0.25) and [0.25, 1).
//
// This predicate has weight of -1 and therefore does not affect route weight.
//
// Example of routes splitting traffic in 50%+30%+20% proportion:
//
//	r50: Path("/test") && TrafficSegment(0.0, 0.5) -> <shunt>;
//	r30: Path("/test") && TrafficSegment(0.5, 0.8) -> <shunt>;
//	r20: Path("/test") && TrafficSegment(0.8, 1.0) -> <shunt>;
func (s *segmentSpec) Create(args []any) (routing.Predicate, error) {
	if len(args) != 2 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	p, ok := &segmentPredicate{randFloat64: s.randFloat64}, false

	if p.min, ok = args[0].(float64); !ok || p.min < 0 || p.min > 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	if p.max, ok = args[1].(float64); !ok || p.max < 0 || p.max > 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	// min == max defines a never-matching interval, e.g. "owl interval" [0,0)
	if p.min > p.max {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	return p, nil
}

// Weight returns -1.
// By returning -1 this predicate does not affect route weight.
func (*segmentSpec) Weight() int {
	return -1
}

func (p *segmentPredicate) Match(req *http.Request) bool {
	r := routing.FromContext(req.Context(), randomValue, p.randFloat64)
	// min == max defines a never-matching interval and always yields false
	return p.min <= r && r < p.max
}
