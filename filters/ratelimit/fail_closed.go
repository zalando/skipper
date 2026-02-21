package ratelimit

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

type failClosedSpec struct{}
type failClosed struct{}
type FailClosedPostProcessor struct{}

func NewFailClosedPostProcessor() *FailClosedPostProcessor {
	return &FailClosedPostProcessor{}
}

// Do is implementing a PostProcessor interface to change the filter
// configs at filter processing time. The fail open/closed decision
// needs to be done once and can be processed before we activate the
// new routes.
func (*FailClosedPostProcessor) Do(routes []*routing.Route) []*routing.Route {
	for _, r := range routes {
		var failClosed bool
		for _, f := range r.Filters {
			if f.Name == filters.RatelimitFailClosedName {
				failClosed = true
				break
			}
		}

		// no config changes detected
		if !failClosed {
			continue
		}

		for _, f := range r.Filters {
			switch f.Name {
			// leaky bucket has no Settings
			case filters.ClusterLeakyBucketRatelimitName:
				lf, ok := f.Filter.(*leakyBucketFilter)
				if ok {
					lf.failClosed = true
				}

			case filters.BackendRateLimitName:
				bf, ok := f.Filter.(*BackendRatelimit)
				if ok {
					bf.Settings.FailClosed = true
				}

			case
				filters.ClientRatelimitName,
				filters.ClusterClientRatelimitName,
				filters.ClusterRatelimitName:

				ff, ok := f.Filter.(*filter)
				if ok {
					ff.settings.FailClosed = true
				}
			}
		}
	}
	return routes
}

func NewFailClosed() filters.Spec {
	return &failClosedSpec{}
}

func (*failClosedSpec) Name() string {
	return filters.RatelimitFailClosedName
}

func (*failClosedSpec) CreateFilter([]any) (filters.Filter, error) {
	return &failClosed{}, nil
}

func (*failClosed) Request(filters.FilterContext) {}

func (*failClosed) Response(filters.FilterContext) {}
