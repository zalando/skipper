package traffic

import "github.com/zalando/skipper/routing"

var ExportRandomValue = randomValue

func WithRandFloat64(ps routing.PredicateSpec, randFloat64 func() float64) routing.PredicateSpec {
	if s, ok := ps.(*spec); ok {
		s.randFloat64 = randFloat64
	} else if s, ok := ps.(*segmentSpec); ok {
		s.randFloat64 = randFloat64
	} else {
		panic("invalid predicate spec, expected *spec or *segmentSpec")
	}
	return ps
}
