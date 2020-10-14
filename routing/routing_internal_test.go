package routing

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper/eskip"
)

func TestSlice(t *testing.T) {
	tests := []struct {
		message string
		r       []*eskip.Route
		offset  int
		limit   int
		expect  []*eskip.Route
	}{
		{
			"empty routes",
			[]*eskip.Route{},
			0,
			0,
			[]*eskip.Route{},
		},
		{
			"to big offset routes",
			[]*eskip.Route{},
			1,
			0,
			[]*eskip.Route{},
		},
		{
			"to big offset and limit routes",
			[]*eskip.Route{},
			1,
			1,
			[]*eskip.Route{},
		},
		{
			"find all in len()=1 with offset 0",
			[]*eskip.Route{{Id: "route1"}},
			0,
			1,
			[]*eskip.Route{{Id: "route1"}},
		},
		{
			"find all in len()=3 with offset 0",
			[]*eskip.Route{{Id: "route1"}, {Id: "route2"}, {Id: "route3"}},
			0,
			3,
			[]*eskip.Route{{Id: "route1"}, {Id: "route2"}, {Id: "route3"}},
		},
		{
			"find all in len()=3 with offset 1",
			[]*eskip.Route{{Id: "route1"}, {Id: "route2"}, {Id: "route3"}},
			1,
			3,
			[]*eskip.Route{{Id: "route2"}, {Id: "route3"}},
		},
		{
			"find all in len()=3 with offset 3",
			[]*eskip.Route{{Id: "route1"}, {Id: "route2"}, {Id: "route3"}},
			3,
			3,
			[]*eskip.Route{},
		},
	}

	for _, ti := range tests {
		t.Run(ti.message, func(t *testing.T) {
			res := slice(ti.r, ti.offset, ti.limit)
			if !cmp.Equal(res, ti.expect) {
				t.Fatalf("Failed test case '%s', got %v and expected %v", ti.message, res, ti.expect)
			}
		})
	}
}

const (
	start = 300
	end   = 700
	step  = 50
)

var goprocs = runtime.GOMAXPROCS(0)

type routingAV struct {
	routeTable atomic.Value // of struct routeTable
}

// Note: load and users increment are not in the critical section
func BenchmarkRoutingGetAtomicValue(b *testing.B) {
	r := &routingAV{}
	rt := &routeTable{}

	r.routeTable.Store(rt)

	for i := start; i < end; i += step {
		b.Run(fmt.Sprintf("goroutines-%d", i*goprocs), func(b *testing.B) {
			b.SetParallelism(i)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					rt := r.routeTable.Load().(*routeTable)
					rt.users.Add(1)
				}
			})
		})
	}
}

type routingMX struct {
	mx         sync.Mutex
	routeTable *routeTable
}

func BenchmarkRoutingGetMutex(b *testing.B) {
	r := &routingMX{}
	r.routeTable = &routeTable{}

	for i := start; i < end; i += step {
		b.Run(fmt.Sprintf("goroutines-%d", i*goprocs), func(b *testing.B) {
			b.SetParallelism(i)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					r.mx.Lock()
					rt := r.routeTable
					rt.users.Add(1)
					r.mx.Unlock()
				}
			})
		})
	}
}

type routingRWMX struct {
	mx         sync.RWMutex
	routeTable *routeTable
}

func BenchmarkRoutingGetRWMutex(b *testing.B) {
	r := &routingRWMX{}
	r.routeTable = &routeTable{}

	for i := start; i < end; i += step {
		b.Run(fmt.Sprintf("goroutines-%d", i*goprocs), func(b *testing.B) {
			b.SetParallelism(i)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					r.mx.RLock()
					rt := r.routeTable
					rt.users.Add(1)
					r.mx.RUnlock()
				}
			})
		})
	}
}
