package routing_test

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing/pathgen"
)

const (
	hostRegexpRate   = 0.66
	pathRegexpRate   = 0.66
	totalHosts       = 15
	totalPathRegexps = 15
)

func hostRegexp(pg *pathgen.PathGenerator) string {
	firstTags := pg.Strs(2, 4, 1, 9)
	domain := pg.Str(6, 24)
	tlds := pg.Strs(3, 18, 2, 4)
	return fmt.Sprintf(
		"(%s)[.]%s[.](%s)",
		strings.Join(firstTags, "|"),
		domain,
		strings.Join(tlds, "|"),
	)
}

func pathRegexp(pg *pathgen.PathGenerator) string {
	contained := pg.Str(3, 9)
	extension := pg.Str(2, 4)
	return fmt.Sprintf(".*%s.*[.]%s", contained, extension)
}

func benchmarkRegexp(b *testing.B, routesCount, concurrency int) {
	pg := pathgen.New(pathgen.PathGeneratorOptions{RandSeed: 42})

	hosts := make([]string, 0, totalHosts)
	for i := 0; i < totalHosts; i++ {
		hosts = append(hosts, hostRegexp(pg))
	}

	pathRegexps := make([]string, 0, totalPathRegexps)
	for i := 0; i < totalPathRegexps; i++ {
		pathRegexps = append(pathRegexps, pathRegexp(pg))
	}

	paths := make([]string, 0, routesCount)
	for i := 0; i < routesCount; i++ {
		paths = append(paths, pg.Next())
	}

	routes := make([]*eskip.Route, 0, routesCount)
	for i := 0; i < routesCount; i++ {
		r := &eskip.Route{
			Id: fmt.Sprintf("route%d", i),
			Filters: []*eskip.Filter{{
				Name: "status",
				Args: []interface{}{200},
			}},
			BackendType: eskip.ShuntBackend,
		}

		if pg.Rnd.Float64() < pathRegexpRate {
			r.PathRegexps = append(
				r.PathRegexps,
				pathRegexps[pg.Rnd.IntN(len(pathRegexps))],
			)
		} else {
			r.Predicates = append(r.Predicates, &eskip.Predicate{
				Name: "Path",
				Args: []interface{}{
					paths[pg.Rnd.IntN(len(paths))],
				},
			})
		}

		if pg.Rnd.Float64() < hostRegexpRate {
			r.HostRegexps = append(
				r.HostRegexps,
				hosts[pg.Rnd.IntN(len(hosts))],
			)
		}
	}

	fr := builtin.MakeRegistry()
	p := proxytest.New(fr, routes...)
	p.Log.Mute()
	defer p.Close()

	b.ResetTimer()
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			for j := 0; j < b.N/concurrency; j++ {
				if b.Failed() {
					wg.Done()
					return
				}

				rsp, err := http.Get(p.URL + paths[pg.Rnd.IntN(len(paths))])
				if err != nil {
					b.Error(err)
					wg.Done()
					return
				}

				rsp.Body.Close()
			}

			wg.Done()
		}()
	}

	wg.Wait()
}

func BenchmarkRegexp(b *testing.B) {
	for routesCount := 1; routesCount <= 1000000; routesCount *= 100 {
		for concurrency := 1; concurrency <= 256; concurrency <<= 4 {
			b.Run(fmt.Sprintf("%d %d", routesCount, concurrency), func(b *testing.B) {
				benchmarkRegexp(b, routesCount, concurrency)
			})
		}
	}
}
