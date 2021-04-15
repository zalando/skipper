package ratelimit

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/ratelimit"
	"github.com/zalando/skipper/routing"
)

const clusterLeakyBucketName = "clusterLeakyBucket"

type leakyBucketSpec struct {
	registry *ratelimit.Registry
}

type leakyBucketFilter struct {
	bucket    *ratelimit.ClusterLeakyBucket
	key       *eskip.Template
	increment int
}

type leakyBucketPreProcessor struct{}

func NewLeakyBucket(registry *ratelimit.Registry) filters.Spec {
	return &leakyBucketSpec{registry}
}

func (s *leakyBucketSpec) Name() string {
	return clusterLeakyBucketName
}

// clusterLeakyBucket("group", "${key template}", "volume/period", [capacity=1, increment=1])
func (s *leakyBucketSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) < 3 || len(args) > 5 {
		return nil, filters.ErrInvalidFilterParameters
	}

	group, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	key, ok := args[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	rate, ok := args[2].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}
	n, d, err := parseRate(rate)
	if err != nil {
		return nil, err
	}

	capacity := 1
	if len(args) > 3 {
		capacity, err = natural(args[3])
		if err != nil {
			return nil, err
		}
	}

	increment := 1
	if len(args) > 4 {
		increment, err = natural(args[4])
		if err != nil {
			return nil, err
		}
	}

	bucket := ratelimit.NewClusterLeakyBucket(s.registry, group, capacity, d/time.Duration(n))

	return &leakyBucketFilter{bucket, eskip.NewTemplate(key), increment}, nil
}

func (f *leakyBucketFilter) Request(ctx filters.FilterContext) {
	key, ok := f.key.ApplyContext(ctx)
	if !ok {
		return // allow on missing placeholders
	}
	allow, retry, err := f.bucket.Check(ctx.Request().Context(), key, f.increment)
	if err != nil {
		return // allow on error
	}
	if allow {
		return
	}
	header := http.Header{}
	if retry > 0 {
		header.Set("Retry-After", strconv.Itoa(int(retry/time.Second)))
	}
	ctx.Serve(&http.Response{StatusCode: http.StatusTooManyRequests, Header: header})
}

func (*leakyBucketFilter) Response(filters.FilterContext) {}

// Parses rate string in "number/duration" format (e.g. "10/m", "1/5s")
func parseRate(rate string) (n int, d time.Duration, err error) {
	s := strings.SplitN(rate, "/", 2)
	if len(s) != 2 {
		err = fmt.Errorf(`rate %s doesn't match the "number/duration" format (e.g. 10/m, 1/5s)`, rate)
		return
	}

	n, err = strconv.Atoi(s[0])
	if err != nil {
		return
	}
	if n <= 0 {
		err = fmt.Errorf(`number %d in rate "number/duration" format must be positive`, n)
		return
	}

	switch s[1] {
	case "ns", "us", "µs", "ms", "s", "m", "h":
		s[1] = "1" + s[1]
	}
	d, err = time.ParseDuration(s[1])
	if err != nil {
		return
	}
	if d <= 0 {
		err = fmt.Errorf(`duration %v in rate "number/duration" format must be positive`, d)
	}
	return
}

func natural(arg interface{}) (n int, err error) {
	switch v := arg.(type) {
	case int:
		n = v
	case float64:
		n = int(v)
	default:
		err = fmt.Errorf(`failed to convert "%v" to integer`, arg)
	}
	if n < 1 {
		err = fmt.Errorf(`number %d must be positive`, n)
	}
	return
}

func NewLeakyBucketPreProcessor() routing.PreProcessor {
	return &leakyBucketPreProcessor{}
}

func (p *leakyBucketPreProcessor) Do(routes []*eskip.Route) []*eskip.Route {
	logProblems(routes)
	return routes
}

// Logs problematic filter configurations.
// Filters with the same group must have the same arguments except increment
func logProblems(routes []*eskip.Route) {
	am := make(map[interface{}]struct {
		routeId string
		args    []interface{}
	})
	for _, route := range routes {
		for _, filter := range route.Filters {
			if filter.Name == clusterLeakyBucketName {
				group := filter.Args[0]
				if config, ok := am[group]; ok {
					if !compatibleArgs(config.args, filter.Args) {
						log.Errorf(`%s filters on route "%s" and "%s" have incompatible arguments: %v vs %v`,
							filter.Name, config.routeId, route.Id, config.args, filter.Args)
					}
				} else {
					am[group] = struct {
						routeId string
						args    []interface{}
					}{route.Id, filter.Args}
				}
			}
		}
	}
}

func compatibleArgs(a, b []interface{}) bool {
	// "group", "${key template}", "volume/period", [capacity=1, increment=1]
	// first 4 args must match, increment may differ
	const N = 4
	if len(a) > N {
		a = a[:N]
	}
	if len(b) > N {
		b = b[:N]
	}
	return reflect.DeepEqual(a, b)
}
