package kubernetes

import (
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/predicates"
)

type trafficSegmentPredicate struct {
	min, max float64
}

func trafficSegmentPredicateCalculator[T definitions.WeightedBackend](b []T) map[string]backendTraffic {
	bt := make(map[string]backendTraffic, len(b))

	sum := 0.0
	for _, bi := range b {
		if _, ok := bt[bi.GetName()].(*trafficSegmentPredicate); ok {
			// ignore duplicate backends
			continue
		}

		p := &trafficSegmentPredicate{}
		bt[bi.GetName()] = p

		p.min = sum
		sum += bi.GetWeight()
		p.max = sum
	}

	if sum == 0 {
		// evenly split traffic between backends
		// range over b instead of bt for stable order
		for _, bi := range b {
			p := bt[bi.GetName()].(*trafficSegmentPredicate)
			p.min = sum
			sum += 1
			p.max = sum
		}
	}

	// normalize segments
	for _, v := range bt {
		p := v.(*trafficSegmentPredicate)

		p.min /= sum

		// last segment always ends up with p.max equal to one because
		// dividing a finite non-zero value by itself always produces one,
		// see https://stackoverflow.com/questions/63439390/does-ieee-754-float-division-or-subtraction-by-itself-always-result-in-the-same
		p.max /= sum
	}
	return bt
}

func (ts *trafficSegmentPredicate) allowed() bool {
	return ts.min != ts.max
}

func (ts *trafficSegmentPredicate) apply(r *eskip.Route) {
	if ts.min == 0 && ts.max == 1 {
		return
	}
	r.Predicates = appendPredicate(r.Predicates, predicates.TrafficSegmentName, ts.min, ts.max)
}
