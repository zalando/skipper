package scheduler_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
			name:    "one scheduler filter fifo",
			doc:     `f2: * -> fifo(10, 12, "10s") -> "http://www.example.org"`,
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
			name:    "multiple filters with one scheduler filter fifo",
			doc:     `f3: * -> setPath("/bar") -> fifo(10, 12, "10s") -> setRequestHeader("X-Foo", "bar") -> "http://www.example.org"`,
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
			name:    "multiple routes with fifo filters do not interfere",
			doc:     `f4: Path("/f4") -> setPath("/bar") -> fifo(10, 12, "10s") -> "http://www.example.org"; l5: Path("/f5") -> setPath("/foo") -> fifo(15, 2, "11s")  -> setRequestHeader("X-Foo", "bar")-> "http://www.example.org";`,
			paths:   [][]string{{"f4"}, {"f5"}},
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
						t.Fatalf("Filter is nil but we do not expect an error")
					}
					if ff, ffOk := f.Filter.(scheduler.FIFOFilter); ffOk {
						cfg := ff.Config()
						queue := ff.GetQueue()
						if queue == nil {
							t.Errorf("FifoQueue is nil")
						}

						if cfg != queue.Config() {
							t.Errorf("Failed to get fifo queue with configuration, want: %v, got: %v", cfg, queue)
						}

						continue
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
			fifoQueuesMap := make(map[string][]*scheduler.FifoQueue)
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
							t.Fatalf("Filter is nil but we do not expect an error")
						}

						if ff, ffOk := f.Filter.(scheduler.FIFOFilter); ffOk {
							cfg := ff.Config()
							queue := ff.GetQueue()
							if queue == nil {
								t.Errorf("FifoQueue is nil")
							}

							if cfg != queue.Config() {
								t.Errorf("Failed to get fifo queue with configuration, want: %v, got: %v", cfg, queue)
							}

							fifoQueuesMap[key] = append(fifoQueuesMap[key], queue)

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

				if len(queuesMap[key]) != len(group) && len(fifoQueuesMap[key]) != len(group) {
					t.Errorf("Failed to get the right group size %v != %v && %v != %v", len(queuesMap[key]), len(group), len(fifoQueuesMap[key]), len(group))
				}
			}
			// check pointers to queue are the same for same group
			for k, queues := range fifoQueuesMap {
				firstQueue := queues[0]
				for _, queue := range queues {
					if queue != firstQueue {
						t.Errorf("Unexpected different fifo queue in group: %s", k)
					}
				}
			}
			for k, queues := range queuesMap {
				firstQueue := queues[0]
				for _, queue := range queues {
					if queue != firstQueue {
						t.Errorf("Unexpected different queue in group: %s", k)
					}
				}
			}
			// check pointers to queue of different groups are different
			diffFifoQueues := make(map[*scheduler.FifoQueue]struct{})
			for _, queues := range fifoQueuesMap {
				diffFifoQueues[queues[0]] = struct{}{}
			}
			if len(diffFifoQueues) != len(fifoQueuesMap) {
				t.Error("Unexpected got pointer to the same fifoqueue for different group")
			}
			diffQueues := make(map[*scheduler.Queue]struct{})
			for _, queues := range queuesMap {
				diffQueues[queues[0]] = struct{}{}
			}
			if len(diffQueues) != len(queuesMap) {
				t.Error("Unexpected got pointer to the same queue for different group")
			}
		})
	}

}

func TestConfig(t *testing.T) {
	waitForStatus := func(t *testing.T, fq *scheduler.FifoQueue, q *scheduler.Queue, s scheduler.QueueStatus) {
		t.Helper()
		var st scheduler.QueueStatus
		timeout := time.After(120 * time.Millisecond)
		for {
			if q != nil {
				st = q.Status()
				if st == s {
					return
				}
			}

			if fq != nil {
				st = fq.Status()
				if st == s {
					return
				}
			}

			select {
			case <-timeout:
				t.Fatalf("failed to reach status got %v, want %v", st, s)
			default:
			}
		}
	}

	const testQueueCloseDelay = 1 * time.Second
	*scheduler.ExportQueueCloseDelay = testQueueCloseDelay

	initTest := func(doc string) (*routing.Routing, *testdataclient.Client, func()) {
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
		return rt, cli, func() {
			rt.Close()
			reg.Close()
		}
	}

	updateDoc := func(t *testing.T, dc *testdataclient.Client, upsertDoc string, deletedIDs []string) {
		t.Helper()
		if err := dc.UpdateDoc(upsertDoc, deletedIDs); err != nil {
			t.Fatal(err)
		}
		time.Sleep(120 * time.Millisecond)
	}

	getQueue := func(path string, rt *routing.Routing) *scheduler.Queue {
		req := &http.Request{URL: &url.URL{Path: path}}
		r, _ := rt.Route(req)
		f := r.Filters[0]
		return f.Filter.(scheduler.LIFOFilter).GetQueue()
	}

	getFifoQueue := func(path string, rt *routing.Routing) *scheduler.FifoQueue {
		req := &http.Request{URL: &url.URL{Path: path}}
		r, _ := rt.Route(req)
		f := r.Filters[0]
		return f.Filter.(scheduler.FIFOFilter).GetQueue()
	}

	t.Run("group config applied", func(t *testing.T) {
		const doc = `
			g1: Path("/one") -> lifoGroup("g", 2, 2) -> <shunt>;
			g2: Path("/two") -> lifoGroup("g") -> <shunt>;
		`

		rt, _, close := initTest(doc)
		defer close()

		req1 := &http.Request{URL: &url.URL{Path: "/one"}}
		req2 := &http.Request{URL: &url.URL{Path: "/two"}}

		r1, _ := rt.Route(req1)
		r2, _ := rt.Route(req2)

		f1 := r1.Filters[0]
		f2 := r2.Filters[0]

		// fill up the group queue:
		go f1.Request(&filtertest.Context{FRequest: req1, FStateBag: make(map[string]any)})
		go f1.Request(&filtertest.Context{FRequest: req1, FStateBag: make(map[string]any)})
		go f2.Request(&filtertest.Context{FRequest: req2, FStateBag: make(map[string]any)})
		go f2.Request(&filtertest.Context{FRequest: req2, FStateBag: make(map[string]any)})

		q1 := f1.Filter.(scheduler.LIFOFilter).GetQueue()
		q2 := f2.Filter.(scheduler.LIFOFilter).GetQueue()

		if q1 != q2 {
			t.Error("the queues in the group don't match")
		}

		waitForStatus(t, nil, q1, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 2})
	})

	t.Run("update fifo config increase concurrency", func(t *testing.T) {
		const doc = `route: * -> fifo(2, 2, "3s") -> <shunt>`
		rt, dc, close := initTest(doc)
		defer close()

		req := &http.Request{URL: &url.URL{}}
		r, _ := rt.Route(req)
		f := r.Filters[0]

		// fill up the queue:
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})

		q := f.Filter.(scheduler.FIFOFilter).GetQueue()
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 2})

		// change the configuration
		updateDoc(t, dc, `route: * -> fifo(3, 2, "3s") -> <shunt>`, nil)
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 0, QueuedRequests: 0})

		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 3, QueuedRequests: 2})
	})

	t.Run("update fifo config increase max queue size", func(t *testing.T) {
		const doc = `route: * -> fifo(2, 2, "3s") -> <shunt>`
		rt, dc, close := initTest(doc)
		defer close()

		req := &http.Request{URL: &url.URL{}}
		r, _ := rt.Route(req)
		f := r.Filters[0]

		// fill up the queue:
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})

		q := f.Filter.(scheduler.FIFOFilter).GetQueue()
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 2})

		// change the configuration
		updateDoc(t, dc, `route: * -> fifo(2, 3, "3s") -> <shunt>`, nil)
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 0, QueuedRequests: 0})

		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 3})
	})

	t.Run("update fifo config decrease max queue size", func(t *testing.T) {
		const doc = `route: * -> fifo(2, 2, "3s") -> <shunt>`
		rt, dc, close := initTest(doc)
		defer close()

		req := &http.Request{URL: &url.URL{}}
		r, _ := rt.Route(req)
		f := r.Filters[0]

		// fill up the queue:
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})

		q := f.Filter.(scheduler.FIFOFilter).GetQueue()
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 1})

		// change the configuration
		updateDoc(t, dc, `route: * -> fifo(2, 1, "3s") -> <shunt>`, nil)

		// update resets
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 0, QueuedRequests: 0})
		//filling again
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 1})

		// adding requests won't change the state if we have already too many in queue
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 1})
	})

	t.Run("update fifo config decrease max concurrency", func(t *testing.T) {
		const doc = `route: * -> fifo(2, 2, "3s") -> <shunt>`
		rt, dc, close := initTest(doc)
		defer close()

		req := &http.Request{URL: &url.URL{}}
		r, _ := rt.Route(req)
		f := r.Filters[0]

		// fill up the queue minus one:
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})

		q := f.Filter.(scheduler.FIFOFilter).GetQueue()
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 1})

		// change the configuration
		updateDoc(t, dc, `route: * -> fifo(1, 2, "3s") -> <shunt>`, nil)

		// update resets
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 0, QueuedRequests: 0})
		// filling again
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 1, QueuedRequests: 2})

		// adding requests won't change the state if we have already too many in queue
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 1, QueuedRequests: 2})
	})

	t.Run("update fifo config decrease max queue size overflow test", func(t *testing.T) {
		const doc = `route: * -> fifo(2, 2, "3s") -> <shunt>`
		rt, dc, close := initTest(doc)
		defer close()

		req := &http.Request{URL: &url.URL{}}
		r, _ := rt.Route(req)
		f := r.Filters[0]

		// fill up the queue:
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})

		q := f.Filter.(scheduler.FIFOFilter).GetQueue()
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 2})

		// change the configuration
		updateDoc(t, dc, `route: * -> fifo(2, 1, "3s") -> <shunt>`, nil)

		// update resets
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 0, QueuedRequests: 0})

		// fill up the queue again
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 1})

		// adding requests won't change the state if we have already too many in queue
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 1})
	})

	t.Run("update fifo config decrease max concurrency overflow test", func(t *testing.T) {
		const doc = `route: * -> fifo(2, 2, "3s") -> <shunt>`
		rt, dc, close := initTest(doc)
		defer close()

		req := &http.Request{URL: &url.URL{}}
		r, _ := rt.Route(req)
		f := r.Filters[0]

		// fill up the queue minus one:
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})

		q := f.Filter.(scheduler.FIFOFilter).GetQueue()
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 2})

		// change the configuration
		updateDoc(t, dc, `route: * -> fifo(1, 2, "3s") -> <shunt>`, nil)

		// update resets
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 0, QueuedRequests: 0})

		// fill up the queue again:
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 1, QueuedRequests: 2})

		// adding requests won't change the state if we have already too many in queue
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		waitForStatus(t, q, nil, scheduler.QueueStatus{ActiveRequests: 1, QueuedRequests: 2})
	})

	t.Run("update lifo config", func(t *testing.T) {
		const doc = `route: * -> lifo(2, 2) -> <shunt>`
		rt, dc, close := initTest(doc)
		defer close()

		req := &http.Request{URL: &url.URL{}}
		r, _ := rt.Route(req)
		f := r.Filters[0]

		// fill up the queue:
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})
		go f.Request(&filtertest.Context{FRequest: req, FStateBag: make(map[string]any)})

		q := f.Filter.(scheduler.LIFOFilter).GetQueue()
		waitForStatus(t, nil, q, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 2})

		// change the configuration, should decrease the queue size:
		updateDoc(t, dc, `route: * -> lifo(2, 1) -> <shunt>`, nil)

		waitForStatus(t, nil, q, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 1})
	})

	t.Run("update group config", func(t *testing.T) {
		const doc = `
			g1: Path("/one") -> lifoGroup("g", 2, 2) -> <shunt>;
			g2: Path("/two") -> lifoGroup("g") -> <shunt>;
		`

		rt, dc, close := initTest(doc)
		defer close()

		req1 := &http.Request{URL: &url.URL{Path: "/one"}}
		req2 := &http.Request{URL: &url.URL{Path: "/two"}}

		r1, _ := rt.Route(req1)
		r2, _ := rt.Route(req2)

		f1 := r1.Filters[0]
		f2 := r2.Filters[0]

		// fill up the group queue:
		go f1.Request(&filtertest.Context{FRequest: req1, FStateBag: make(map[string]any)})
		go f1.Request(&filtertest.Context{FRequest: req1, FStateBag: make(map[string]any)})
		go f2.Request(&filtertest.Context{FRequest: req2, FStateBag: make(map[string]any)})
		go f2.Request(&filtertest.Context{FRequest: req2, FStateBag: make(map[string]any)})

		q := f1.Filter.(scheduler.LIFOFilter).GetQueue()
		waitForStatus(t, nil, q, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 2})

		// change the configuration, should decrease the queue size:
		updateDoc(t, dc, `
			g1: Path("/one") -> lifoGroup("g", 2, 1) -> <shunt>;
			g2: Path("/two") -> lifoGroup("g") -> <shunt>;
		`, nil)

		waitForStatus(t, nil, q, scheduler.QueueStatus{ActiveRequests: 2, QueuedRequests: 1})
	})

	t.Run("queue gets closed when removed after delay", func(t *testing.T) {
		const doc = `
			g1: Path("/one") -> lifo(2, 2) -> <shunt>;
			g2: Path("/two") -> lifo(2, 2) -> <shunt>;
			fq: Path("/fifo") -> fifo(2, 2, "3s") -> <shunt>;
		`

		rt, dc, close := initTest(doc)
		defer close()

		q1 := getQueue("/one", rt)
		q2 := getQueue("/two", rt)
		fq := getFifoQueue("/fifo", rt)
		t.Logf("fq: %v", fq)

		waitForStatus(t, nil, q1, scheduler.QueueStatus{Closed: false})
		waitForStatus(t, nil, q2, scheduler.QueueStatus{Closed: false})
		waitForStatus(t, fq, nil, scheduler.QueueStatus{Closed: false})

		updateDoc(t, dc, "", []string{"g1"})

		// Queue is not closed immediately when deleted
		waitForStatus(t, nil, q1, scheduler.QueueStatus{Closed: false})
		waitForStatus(t, nil, q2, scheduler.QueueStatus{Closed: false})
		waitForStatus(t, fq, nil, scheduler.QueueStatus{Closed: false})

		// An update triggers closing of the deleted queue if it
		// was deleted more than testQueueCloseDelay ago
		time.Sleep(testQueueCloseDelay)
		updateDoc(t, dc, `g3: Path("/three") -> lifo(2, 2) -> <shunt>;`, nil)

		q3 := getQueue("/three", rt)

		waitForStatus(t, nil, q1, scheduler.QueueStatus{Closed: true})
		waitForStatus(t, nil, q2, scheduler.QueueStatus{Closed: false})
		waitForStatus(t, nil, q3, scheduler.QueueStatus{Closed: false})
		waitForStatus(t, fq, nil, scheduler.QueueStatus{Closed: false})
	})
}

func TestRegistryPreProcessor(t *testing.T) {
	fr := builtin.MakeRegistry()

	for _, tc := range []struct {
		name, input, expect string
	}{
		{
			name:   "no lifo",
			input:  `* -> setPath("/foo") -> <shunt>`,
			expect: `* -> setPath("/foo") -> <shunt>`,
		},
		{
			name:   "one lifo",
			input:  `* -> lifo() -> setPath("/foo") -> <shunt>`,
			expect: `* -> lifo() -> setPath("/foo") -> <shunt>`,
		},
		{
			name:   "one fifo",
			input:  `* -> fifo(2, 2, "3s") -> setPath("/foo") -> <shunt>`,
			expect: `* -> fifo(2, 2, "3s") -> setPath("/foo") -> <shunt>`,
		},
		{
			name:   "two lifos",
			input:  `* -> lifo(777) -> lifo() -> setPath("/foo") -> <shunt>`,
			expect: `* -> lifo() -> setPath("/foo") -> <shunt>`,
		},
		{
			name:   "two fifos",
			input:  `* -> fifo(2, 2, "3s") -> fifo(20, 2, "3s") -> setPath("/foo") -> <shunt>`,
			expect: `* -> fifo(20, 2, "3s") -> setPath("/foo") -> <shunt>`,
		},
		{
			name:   "three lifos",
			input:  `* -> lifo(777) -> setPath("/foo") -> lifo(999) -> lifo() -> setPath("/bar") -> <shunt>`,
			expect: `* -> setPath("/foo") -> lifo() -> setPath("/bar") -> <shunt>`,
		},
		{
			name:   "ignores lifoGroup",
			input:  `* -> lifo(777) -> lifoGroup("g") -> lifo(999) -> lifo() -> setPath("/bar") -> <shunt>`,
			expect: `* -> lifoGroup("g") -> lifo() -> setPath("/bar") -> <shunt>`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dc, err := testdataclient.NewDoc(tc.input)
			require.NoError(t, err)

			reg := scheduler.RegistryWith(scheduler.Options{})
			defer reg.Close()

			ro := routing.Options{
				SignalFirstLoad: true,
				FilterRegistry:  fr,
				DataClients:     []routing.DataClient{dc},
				PreProcessors:   []routing.PreProcessor{reg.PreProcessor()},
				PostProcessors:  []routing.PostProcessor{reg},
			}

			rt := routing.New(ro)
			defer rt.Close()

			<-rt.FirstLoad()

			req, _ := http.NewRequest("GET", "http://skipper.test", nil)
			route, _ := rt.Route(req)

			assert.Equal(t, tc.expect, route.String())
		})
	}
}
