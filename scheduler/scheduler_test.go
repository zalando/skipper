package scheduler_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/scheduler"
)

func TestScheduler(t *testing.T) {
	fr := builtin.MakeRegistry()

	for _, tt := range []struct {
		name    string
		doc     string
		paths   [][]string
		wantErr bool
	}{
		{
			name:    "no filter",
			doc:     `r0: * -> "http://www.example.org"`,
			wantErr: true,
		},
		{
			name:    "one filter without scheduler filter",
			doc:     `r1: * -> setPath("/bar") -> "http://www.example.org"`,
			wantErr: false,
		},
		{
			name:    "one scheduler filter lifo",
			doc:     `l2: * -> lifo(10, 12, "10s") -> "http://www.example.org"`,
			wantErr: false,
		},
		{
			name:    "one scheduler filter lifoGroup",
			doc:     `r2: * -> lifoGroup("r2", 10, 12, "10s") -> "http://www.example.org"`,
			wantErr: false,
		},
		{
			name:    "multiple filters with one scheduler filter lifo",
			doc:     `l3: * -> setPath("/bar") -> lifo(10, 12, "10s") -> setRequestHeader("X-Foo", "bar") -> "http://www.example.org"`,
			wantErr: false,
		},
		{
			name:    "multiple filters with one scheduler filter lifoGroup",
			doc:     `r3: * -> setPath("/bar") -> lifoGroup("r3", 10, 12, "10s") -> setRequestHeader("X-Foo", "bar") -> "http://www.example.org"`,
			wantErr: false,
		},
		{
			name:    "multiple routes with lifo filters do not interfere",
			doc:     `l4: Path("/l4") -> setPath("/bar") -> lifo(10, 12, "10s") -> "http://www.example.org"; l5: Path("/l5") -> setPath("/foo") -> lifo(15, 2, "11s")  -> setRequestHeader("X-Foo", "bar")-> "http://www.example.org";`,
			paths:   [][]string{{"l4"}, {"l5"}},
			wantErr: false,
		},
		{
			name:    "multiple routes with different grouping do not interfere",
			doc:     `r4: Path("/r4") -> setPath("/bar") -> lifoGroup("r4", 10, 12, "10s") -> "http://www.example.org"; r5: Path("/r5") -> setPath("/foo") -> lifoGroup("r5", 15, 2, "11s")  -> setRequestHeader("X-Foo", "bar")-> "http://www.example.org";`,
			paths:   [][]string{{"r4"}, {"r5"}},
			wantErr: false,
		},
		{
			name:    "multiple routes with same grouping do use the same configuration",
			doc:     `r6: Path("/r6") -> setPath("/bar") -> lifoGroup("r6", 10, 12, "10s") -> "http://www.example.org"; r7: Path("/r7") -> setPath("/foo") -> lifoGroup("r6", 10, 12, "10s")  -> setRequestHeader("X-Foo", "bar")-> "http://www.example.org";`,
			wantErr: false,
			paths:   [][]string{{"r6", "r7"}},
		}} {
		t.Run(tt.name, func(t *testing.T) {
			cli, err := testdataclient.NewDoc(tt.doc)
			if err != nil {
				t.Fatalf("Failed to create a test dataclient: %v", err)
			}

			reg := scheduler.NewRegistry()
			ro := routing.Options{
				SignalFirstLoad: true,
				FilterRegistry:  fr,
				DataClients:     []routing.DataClient{cli},
				PostProcessors: []routing.PostProcessor{
					reg,
				},
			}
			rt := routing.New(ro)
			defer rt.Close()
			<-rt.FirstLoad() // sync

			if len(tt.paths) == 0 {
				r, _ := rt.Route(&http.Request{URL: &url.URL{Path: "/foo"}})
				if r == nil {
					t.Errorf("Route is nil but we do not expect an error")
					return
				}

				for _, f := range r.Filters {
					if f == nil && !tt.wantErr {
						t.Errorf("Filter is nil but we do not expect an error")
					}
					lf, ok := f.Filter.(scheduler.LIFOFilter)
					if !ok {
						continue
					}
					cfg := lf.Config()
					queue := lf.GetQueue()
					if queue == nil {
						t.Errorf("Queue is nil")
					}
					if cfg != queue.Config() {
						t.Errorf("Failed to get queue with configuration, want: %v, got: %v", cfg, queue)
					}
				}
			}

			queuesMap := make(map[string][]*scheduler.Queue)
			for _, group := range tt.paths {
				key := group[0]

				for _, p := range group {
					r, _ := rt.Route(&http.Request{URL: &url.URL{Path: "/" + p}})
					if r == nil {
						t.Errorf("Route is nil but we do not expect an error, path: %s", p)
						return
					}

					for _, f := range r.Filters {
						if f == nil && !tt.wantErr {
							t.Errorf("Filter is nil but we do not expect an error")
						}

						lf, ok := f.Filter.(scheduler.LIFOFilter)
						if !ok {
							continue
						}

						cfg := lf.Config()
						queue := lf.GetQueue()
						if queue == nil {
							t.Errorf("Queue is nil")
						}

						if cfg != queue.Config() {
							t.Errorf("Failed to get queue with configuration, want: %v, got: %v", cfg, queue)
						}

						queuesMap[key] = append(queuesMap[key], queue)
					}
				}

				if len(queuesMap[key]) != len(group) {
					t.Errorf("Failed to get the right group size %v != %v", len(queuesMap[key]), len(group))
				}
			}
			// check pointers to queue are the same for same group
			for k, stacks := range queuesMap {
				firstQueue := stacks[0]
				for _, queue := range stacks {
					if queue != firstQueue {
						t.Errorf("Unexpected different queue in group: %s", k)
					}
				}
			}
			// check pointers to queue of different groups are different
			diffQueues := make(map[*scheduler.Queue]struct{})
			for _, stacks := range queuesMap {
				diffQueues[stacks[0]] = struct{}{}
			}
			if len(diffQueues) != len(queuesMap) {
				t.Error("Unexpected got pointer to the same queue for different group")
			}
		})
	}

}

func TestConfig(t *testing.T) {
	waitForStatus := func(t *testing.T, q *scheduler.Queue, s scheduler.QueueStatus) {
		timeout := time.After(120 * time.Millisecond)
		for {
			if q.Status() == s {
				return
			}

			select {
			case <-timeout:
				t.Fatal("failed to reach status")
			default:
			}
		}
	}

	initTest := func(doc string) (*scheduler.Registry, *routing.Routing, *testdataclient.Client) {
		cli, err := testdataclient.NewDoc(doc)
		if err != nil {
			t.Fatalf("Failed to create a test dataclient: %v", err)
		}

		reg := scheduler.NewRegistry()
		ro := routing.Options{
			SignalFirstLoad: true,
			FilterRegistry:  builtin.MakeRegistry(),
			DataClients:     []routing.DataClient{cli},
			PostProcessors: []routing.PostProcessor{
				reg,
			},
		}

		rt := routing.New(ro)
		<-rt.FirstLoad()
		return reg, rt, cli
	}

	t.Run("group config applied", func(t *testing.T) {
		const doc = `
			g1: Path("/one") -> lifoGroup("g", 2, 2) -> <shunt>;
			g2: Path("/two") -> lifoGroup("g") -> <shunt>;
		`

		reg, rt, _ := initTest(doc)
		defer reg.Close()
		defer rt.Close()

		req1 := &http.Request{URL: &url.URL{Path: "/one"}}
		req2 := &http.Request{URL: &url.URL{Path: "/two"}}

		r1, _ := rt.Route(req1)
		r2, _ := rt.Route(req2)

		f1 := r1.Filters[0]
		f2 := r2.Filters[0]

		// fill up the group queue:
		ctx1 := &filtertest.Context{FRequest: req1, FStateBag: make(map[string]interface{})}
		ctx2 := &filtertest.Context{FRequest: req2, FStateBag: make(map[string]interface{})}
		go f1.Request(ctx1)
		go f1.Request(ctx1)
		go f2.Request(ctx2)
		go f2.Request(ctx2)

		q1 := f1.Filter.(scheduler.LIFOFilter).GetQueue()
		q2 := f2.Filter.(scheduler.LIFOFilter).GetQueue()

		if q1 != q2 {
			t.Error("the queues in the group don't match")
		}

		waitForStatus(t, q1, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 2})
	})

	t.Run("update config", func(t *testing.T) {
		const doc = `route: * -> lifo(2, 2) -> <shunt>`
		reg, rt, dc := initTest(doc)
		defer reg.Close()
		defer rt.Close()

		req := &http.Request{URL: &url.URL{}}
		r, _ := rt.Route(req)
		f := r.Filters[0]

		// fill up the queue:
		ctx := &filtertest.Context{FRequest: req, FStateBag: make(map[string]interface{})}
		go f.Request(ctx)
		go f.Request(ctx)
		go f.Request(ctx)
		go f.Request(ctx)

		q := f.Filter.(scheduler.LIFOFilter).GetQueue()
		waitForStatus(t, q, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 2})

		// change the configuration, should decrease the queue size:
		const updateDoc = `route: * -> lifo(2, 1) -> <shunt>`
		if err := dc.UpdateDoc(updateDoc, nil); err != nil {
			t.Fatal(err)
		}

		waitForStatus(t, q, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 1})
	})

	t.Run("update group config", func(t *testing.T) {
		const doc = `
			g1: Path("/one") -> lifoGroup("g", 2, 2) -> <shunt>;
			g2: Path("/two") -> lifoGroup("g") -> <shunt>;
		`

		reg, rt, dc := initTest(doc)
		defer reg.Close()
		defer rt.Close()

		req1 := &http.Request{URL: &url.URL{Path: "/one"}}
		req2 := &http.Request{URL: &url.URL{Path: "/two"}}

		r1, _ := rt.Route(req1)
		r2, _ := rt.Route(req2)

		f1 := r1.Filters[0]
		f2 := r2.Filters[0]

		// fill up the group queue:
		ctx1 := &filtertest.Context{FRequest: req1, FStateBag: make(map[string]interface{})}
		ctx2 := &filtertest.Context{FRequest: req2, FStateBag: make(map[string]interface{})}
		go f1.Request(ctx1)
		go f1.Request(ctx1)
		go f2.Request(ctx2)
		go f2.Request(ctx2)

		q := f1.Filter.(scheduler.LIFOFilter).GetQueue()
		waitForStatus(t, q, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 2})

		// change the configuration, should decrease the queue size:
		const updateDoc = `
			g1: Path("/one") -> lifoGroup("g", 2, 1) -> <shunt>;
			g2: Path("/two") -> lifoGroup("g") -> <shunt>;
		`

		if err := dc.UpdateDoc(updateDoc, nil); err != nil {
			t.Fatal(err)
		}

		waitForStatus(t, q, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 1})
	})
}
