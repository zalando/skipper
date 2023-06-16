package kubernetes

import (
	"fmt"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/predicates"
)

// BackendTrafficAlgorithm specifies the algorithm for backend traffic calculation
type BackendTrafficAlgorithm int

const (
	// TrafficPredicateAlgorithm is the default algorithm for backend traffic calculation.
	// It uses Traffic and True predicates to distribute traffic between backends.
	TrafficPredicateAlgorithm BackendTrafficAlgorithm = iota

	// TrafficSegmentPredicateAlgorithm uses TrafficSegment predicate to distribute traffic between backends
	TrafficSegmentPredicateAlgorithm
)

func (a BackendTrafficAlgorithm) String() string {
	switch a {
	case TrafficPredicateAlgorithm:
		return "traffic-predicate"
	case TrafficSegmentPredicateAlgorithm:
		return "traffic-segment-predicate"
	default:
		return "unknown" // should never happen
	}
}

// ParseBackendTrafficAlgorithm parses a string into a BackendTrafficAlgorithm
func ParseBackendTrafficAlgorithm(name string) (BackendTrafficAlgorithm, error) {
	switch name {
	case "traffic-predicate":
		return TrafficPredicateAlgorithm, nil
	case "traffic-segment-predicate":
		return TrafficSegmentPredicateAlgorithm, nil
	default:
		return -1, fmt.Errorf("invalid backend traffic algorithm: %s", name)
	}
}

// backendTraffic specifies whether a given backend is allowed to receive any traffic and
// modifies route to receive the desired traffic portion
type backendTraffic interface {
	allowed() bool
	apply(*eskip.Route)
}

// getBackendTrafficCalculator returns a function that calculates backendTraffic for each backend using specified algorithm
func getBackendTrafficCalculator[T definitions.WeightedBackend](algorithm BackendTrafficAlgorithm) func(b []T) map[string]backendTraffic {
	switch algorithm {
	case TrafficSegmentPredicateAlgorithm:
		return trafficSegmentPredicateCalculator[T]
	case TrafficPredicateAlgorithm:
		return trafficPredicateCalculator[T]
	}
	return nil // should never happen
}

// trafficPredicate implements backendTraffic using Traffic() and True() predicates
type trafficPredicate struct {
	value   float64
	balance int
}

var _ backendTraffic = &trafficPredicate{}

// trafficPredicateCalculator calculates argument for the Traffic() predicate and
// the number of True() predicates to be added to the routes based on the weights of the backends.
//
// The Traffic() argument is calculated based on the following rules:
//
//   - if no weight is defined for a backend it will get weight 0.
//   - if no weights are specified for all backends of a path, then traffic will
//     be distributed equally.
//
// Each Traffic() argument is relative to the number of remaining backends,
// e.g. if the weight is specified as:
//
//	backend-1: 0.1
//	backend-2: 0.2
//	backend-3: 0.3
//	backend-4: 0.4
//
// then Traffic() predicate arguments will be:
//
//	backend-1: Traffic(0.1)   == 0.1 / (0.1 + 0.2 + 0.3 + 0.4)
//	backend-2: Traffic(0.222) == 0.2 / (0.2 + 0.3 + 0.4)
//	backend-3: Traffic(0.428) == 0.3 / (0.3 + 0.4)
//	backend-4: Traffic(1.0)   == 0.4 / (0.4)
//
// The weight of the backend routes will be adjusted by a number of True() predicates
// equal to the number of remaining backends minus one, e.g. for the above example:
//
//	backend-1: Traffic(0.1)   && True() && True() -> ...
//	backend-2: Traffic(0.222) && True() -> ...
//	backend-3: Traffic(0.428) -> ...
//	backend-4: Traffic(1.0)   -> ...
//
// Traffic(1.0) is handled in a special way, see trafficPredicate.apply().
func trafficPredicateCalculator[T definitions.WeightedBackend](b []T) map[string]backendTraffic {
	sum := 0.0
	weights := make([]float64, len(b))
	for i, bi := range b {
		w := bi.GetWeight()
		weights[i] = w
		sum += w
	}

	if sum == 0 {
		sum = float64(len(weights))
		for i := range weights {
			weights[i] = 1
		}
	}

	var lastWithWeight int
	for i, w := range weights {
		if w > 0 {
			lastWithWeight = i
		}
	}

	bt := make(map[string]backendTraffic)

	for i, bi := range b {
		ct := &trafficPredicate{}
		bt[bi.GetName()] = ct
		switch {
		case i == lastWithWeight:
			ct.value = 1
		case weights[i] == 0:
			ct.value = 0
		default:
			ct.value = weights[i] / sum
		}

		sum -= weights[i]
		ct.balance = len(b) - i - 2
	}

	return bt
}

func (tp *trafficPredicate) allowed() bool {
	return tp.value > 0
}

// apply adds Traffic() and True() predicates to the route.
// For the value of 1.0 no predicates will be added.
func (tp *trafficPredicate) apply(r *eskip.Route) {
	if tp.value == 1.0 {
		return
	}

	r.Predicates = appendPredicate(r.Predicates, predicates.TrafficName, tp.value)
	for i := 0; i < tp.balance; i++ {
		r.Predicates = appendPredicate(r.Predicates, predicates.TrueName)
	}
}
