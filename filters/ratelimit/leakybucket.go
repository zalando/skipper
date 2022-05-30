package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/ratelimit"
)

type leakyBucket interface {
	Add(ctx context.Context, label string, increment int) (added bool, retry time.Duration, err error)
}

type leakyBucketSpec struct {
	create func(capacity int, emission time.Duration) leakyBucket
}

type leakyBucketFilter struct {
	label     *eskip.Template
	bucket    leakyBucket
	increment int
}

func NewClusterLeakyBucket(registry *ratelimit.Registry) filters.Spec {
	return &leakyBucketSpec{
		create: func(capacity int, emission time.Duration) leakyBucket {
			return ratelimit.NewClusterLeakyBucket(registry, capacity, emission)
		},
	}
}

func (s *leakyBucketSpec) Name() string {
	return filters.ClusterLeakyBucketRatelimitName
}

// clusterLeakyBucketRatelimit("a-label-${template}", leakVolume, "leak period", capacity, increment)
func (s *leakyBucketSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 5 {
		return nil, filters.ErrInvalidFilterParameters
	}

	label, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	leakVolume, err := natural(args[1])
	if err != nil {
		return nil, err
	}

	leakPeriod, err := getDurationArg(args[2])
	if err != nil {
		return nil, err
	}
	if leakPeriod <= 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	capacity, err := natural(args[3])
	if err != nil {
		return nil, err
	}

	increment, err := natural(args[4])
	if err != nil {
		return nil, err
	}

	// emission is the reciprocal of the leak rate
	emission := leakPeriod / time.Duration(leakVolume)

	return &leakyBucketFilter{eskip.NewTemplate(label), s.create(capacity, emission), increment}, nil
}

func (f *leakyBucketFilter) Request(ctx filters.FilterContext) {
	label, ok := f.label.ApplyContext(ctx)
	if !ok {
		return // allow on missing placeholders
	}
	added, retry, err := f.bucket.Add(ctx.Request().Context(), label, f.increment)
	if err != nil {
		return // allow on error
	}
	if added {
		return // allow if successfully added
	}

	header := http.Header{}
	if retry > 0 {
		header.Set("Retry-After", strconv.Itoa(int(retry/time.Second)))
	}
	ctx.Serve(&http.Response{StatusCode: http.StatusTooManyRequests, Header: header})
}

func (*leakyBucketFilter) Response(filters.FilterContext) {}

func natural(arg interface{}) (n int, err error) {
	n, err = getIntArg(arg)
	if err == nil && n <= 0 {
		err = fmt.Errorf(`number %d must be positive`, n)
	}
	return
}
