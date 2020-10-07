package predicates

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/predicates/traffic"
	"github.com/zalando/skipper/routing"
)

func Path(path string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: routing.PathName,
		Args: []interface{}{path},
	}
}

func Traffic(chance float64) *eskip.Predicate {
	return &eskip.Predicate{
		Name: traffic.PredicateName,
		Args: []interface{}{chance},
	}
}

func TrafficSticky(chance float64, trafficGroupCookie, trafficGroup string) *eskip.Predicate {
	return &eskip.Predicate{
		Name: traffic.PredicateName,
		Args: []interface{}{chance, trafficGroupCookie, trafficGroup},
	}
}
