package shedder

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

func TestAdmissionControl(t *testing.T) {
	// fixed rand to have non flaky tests
	rand := rand.New(rand.NewSource(5))

	for _, ti := range []struct {
		msg                        string
		mode                       string
		d                          time.Duration
		windowsize                 int
		minRequests                int
		successThreshold           float64
		maxrejectprobability       float64
		exponent                   float64
		N                          int     // iterations
		pBackendErr                float64 // [0,1]
		pExpectedAdmissionShedding float64 // [0,1]
	}{{
		msg:                        "no error",
		mode:                       "active",
		d:                          10 * time.Millisecond,
		windowsize:                 5,
		minRequests:                10,
		successThreshold:           0.9,
		maxrejectprobability:       0.95,
		exponent:                   1.0,
		N:                          20,
		pBackendErr:                0.0,
		pExpectedAdmissionShedding: 0.0,
	}, {
		msg:                        "only errors",
		mode:                       "active",
		d:                          10 * time.Millisecond,
		windowsize:                 5,
		minRequests:                10,
		successThreshold:           0.9,
		maxrejectprobability:       0.95,
		exponent:                   1.0, // 1000.0
		N:                          20,
		pBackendErr:                1.0,
		pExpectedAdmissionShedding: 0.95,
	}, {
		msg:                        "smaller error rate, than threshold won't block",
		mode:                       "active",
		d:                          10 * time.Millisecond,
		windowsize:                 5,
		minRequests:                10,
		successThreshold:           0.9,
		maxrejectprobability:       0.95,
		exponent:                   1.0,
		N:                          20,
		pBackendErr:                0.01,
		pExpectedAdmissionShedding: 0.0,
	}, {
		msg:                        "tiny error rate and bigger than threshold will block some traffic",
		mode:                       "active",
		d:                          10 * time.Millisecond,
		windowsize:                 5,
		minRequests:                10,
		successThreshold:           0.99,
		maxrejectprobability:       0.95,
		exponent:                   1.0,
		N:                          20,
		pBackendErr:                0.1,
		pExpectedAdmissionShedding: 0.1,
	}, {
		msg:                        "small error rate and bigger than threshold will block some traffic",
		mode:                       "active",
		d:                          10 * time.Millisecond,
		windowsize:                 5,
		minRequests:                10,
		successThreshold:           0.9,
		maxrejectprobability:       0.95,
		exponent:                   1.0,
		N:                          20,
		pBackendErr:                0.2,
		pExpectedAdmissionShedding: 0.1,
	}, {
		msg:                        "medium error rate and bigger than threshold will block some traffic",
		mode:                       "active",
		d:                          10 * time.Millisecond,
		windowsize:                 5,
		minRequests:                10,
		successThreshold:           0.9,
		maxrejectprobability:       0.95,
		exponent:                   1.0,
		N:                          20,
		pBackendErr:                0.5,
		pExpectedAdmissionShedding: 0.615,
	}, {
		msg:                        "large error rate and bigger than threshold will block some traffic",
		mode:                       "active",
		d:                          10 * time.Millisecond,
		windowsize:                 5,
		minRequests:                10,
		successThreshold:           0.9,
		maxrejectprobability:       0.95,
		exponent:                   1.0,
		N:                          20,
		pBackendErr:                0.8,
		pExpectedAdmissionShedding: 0.91,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				p := rand.Float64()
				if p < ti.pBackendErr {
					w.WriteHeader(http.StatusInternalServerError)
				} else {
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer backend.Close()

			spec := NewAdmissionControl(Options{}).(*AdmissionControlSpec)

			args := make([]interface{}, 0, 6)
			args = append(args, "testmetric", ti.mode, ti.d.String(), ti.windowsize, ti.minRequests, ti.successThreshold, ti.maxrejectprobability, ti.exponent)

			f, err := spec.CreateFilter(args)
			if err != nil {
				t.Logf("args: %+v", args...)
				t.Fatalf("error creating filter: %v", err)
				return
			}
			defer f.(*admissionControl).Close()

			fr := make(filters.Registry)
			fr.Register(spec)
			r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}

			dc := testdataclient.New([]*eskip.Route{r})
			defer dc.Close()

			proxy := proxytest.WithRoutingOptions(fr, routing.Options{
				DataClients:    []routing.DataClient{dc},
				PostProcessors: []routing.PostProcessor{spec.PostProcessor()},
				PreProcessors:  []routing.PreProcessor{spec.PreProcessor()},
			})
			defer proxy.Close()

			client := proxy.Client()
			req, err := http.NewRequest("GET", proxy.URL, nil)
			if err != nil {
				t.Error(err)
				return
			}

			var failBackend, fail, ok, N float64
			// iterations to make sure we have enough traffic
			until := time.After(time.Duration(ti.N) * time.Duration(ti.windowsize) * ti.d)
		work:
			for {
				select {
				case <-until:
					break work
				default:
				}
				N++
				rsp, err := client.Do(req)
				if err != nil {
					t.Error(err)
				}
				switch rsp.StatusCode {
				case http.StatusInternalServerError:
					failBackend += 1
				case http.StatusServiceUnavailable:
					fail += 1
				case http.StatusOK:
					ok += 1
				default:
					t.Logf("Unexpected status code %d %s", rsp.StatusCode, rsp.Status)
				}
				rsp.Body.Close()
			}
			t.Logf("ok: %0.2f, fail: %0.2f, failBackend: %0.2f", ok, fail, failBackend)

			epsilon := 0.05 * N // maybe 5% instead of 0.1%
			expectedFails := (N - failBackend) * ti.pExpectedAdmissionShedding

			if expectedFails-epsilon > fail || fail > expectedFails+epsilon {
				t.Errorf("Failed to get expected fails should be in: %0.2f < %0.2f < %0.2f", expectedFails-epsilon, fail, expectedFails+epsilon)
			}

			// TODO(sszuecs) how to calculate expected oks?
			// expectedOKs := N - (N-failBackend)*ti.pExpectedAdmissionShedding
			// if ok < expectedOKs-epsilon || expectedOKs+epsilon < ok {
			// 	t.Errorf("Failed to get expected ok should be in: %0.2f < %0.2f < %0.2f", expectedOKs-epsilon, ok, expectedOKs+epsilon)
			// }
		})
	}
}

func TestAdmissionControlCleanupModes(t *testing.T) {
	for _, ti := range []struct {
		msg  string
		mode string
	}{{
		msg:  "cleanup works for active mode",
		mode: active.String(),
	}, {
		msg:  "cleanup works for inactive mode",
		mode: inactive.String(),
	}, {
		msg:  "cleanup works for inactiveLog mode",
		mode: logInactive.String(),
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			ch := make(chan tuple)
			validationPostProcessor := &validationPostProcessorClosedFilter{
				ch: ch,
			}
			backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer backend1.Close()

			backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer backend2.Close()

			fspec := NewAdmissionControl(Options{})
			spec, ok := fspec.(*AdmissionControlSpec)
			if !ok {
				t.Fatal("FilterSpec is not a AdmissionControlSpec")
			}
			preProcessor := spec.PreProcessor()
			postProcessor := spec.PostProcessor()

			args := make([]interface{}, 0, 6)
			args = append(args, "testmetric", ti.mode, "10ms", 5, 1, 0.1, 0.95, 0.5)
			f, err := spec.CreateFilter(args)
			if err != nil {
				t.Fatalf("error creating filter: %v", err)
				return
			}
			f.(*admissionControl).Close()

			fr := make(filters.Registry)
			fr.Register(spec)

			r1 := &eskip.Route{
				Id:      "r1",
				Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}},
				Backend: backend1.URL,
			}
			r2 := &eskip.Route{
				Id:      "r2",
				Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}},
				Backend: backend2.URL,
			}

			dc := testdataclient.New([]*eskip.Route{r1})
			defer dc.Close()

			proxy := proxytest.WithRoutingOptions(fr, routing.Options{
				DataClients:    []routing.DataClient{dc},
				PostProcessors: []routing.PostProcessor{postProcessor, validationPostProcessor},
				PreProcessors:  []routing.PreProcessor{preProcessor},
			})
			defer proxy.Close()

			var tuple tuple
			var deletedIDs []string

			// first does not need update, it's the dataclient triggered load that runs the processors
			tuple = <-ch
			if tuple.id != r1.Id {
				t.Fatalf("Failed to get route got: %s", tuple.id)
			}

			// delete route triggers closing filter, add r2 works
			deletedIDs = []string{r1.Id}
			dc.Update([]*eskip.Route{r2}, deletedIDs)
			tuple = <-ch
			if tuple.id != r2.Id {
				t.Fatalf("Failed to get route %s, got: %s", r2.Id, tuple.id)
			}
			if !tuple.closed {
				t.Errorf("Deleted filter should be closed routeID: %s", deletedIDs[0])
			}

			// preProcessor will only apply one filter in r2 (last wins)
			r2.Filters = append(r2.Filters, r2.Filters...)
			dc.Update([]*eskip.Route{r1, r2}, nil)
			tuple = <-ch
			tuple2 := <-ch
			if tuple2.id == r2.Id {
				tuple = tuple2
			}
			if tuple.id != r2.Id {
				t.Fatalf("Failed to cleanup preprocessor %s should be there", r2.Id)
			}
			// reset r2
			r2.Filters = []*eskip.Filter{r2.Filters[0]}

			// delete r2 triggers closing and r1 exists
			deletedIDs = []string{r2.Id}
			dc.Update([]*eskip.Route{}, deletedIDs)
			tuple = <-ch
			if tuple.id != r1.Id {
				t.Fatalf("Failed to delete route %s, got: '%q'", r2.Id, tuple.id)
			}
			if !tuple.closed {
				t.Error("old filter should be closed")
			}

			// delete r1 triggers closing
			deletedIDs = []string{r1.Id}
			dc.Update([]*eskip.Route{}, deletedIDs)
			tuple = <-ch
			if !tuple.closed {
				t.Error("old filter should be closed")
			}
		})
	}
}

type tuple struct {
	id     string
	closed bool
}
type validationPostProcessorClosedFilter struct {
	ch        chan tuple
	oldFilter *admissionControl
}

// Do validates if admissioncontrol filter exists in at least one
// route. It sends a tuple of route ID and closed state of found
// admissionControl filter through the channel. Empty string if not
// found. True if closed.
func (vpp *validationPostProcessorClosedFilter) Do(routes []*routing.Route) []*routing.Route {
	found := false

	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Id < routes[j].Id
	})
	for _, r := range routes {
		for _, f := range r.Filters {
			if ac, ok := f.Filter.(*admissionControl); ok {
				found = true

				closed := false
				if vpp.oldFilter != nil {
					closed = vpp.oldFilter.closed
				}
				go func(id string, b bool) {
					vpp.ch <- tuple{id: id, closed: closed}
				}(r.Id, closed)

				vpp.oldFilter = ac
			}
		}
	}

	if !found {
		go func() { vpp.ch <- tuple{id: "", closed: vpp.oldFilter.closed} }()
	}
	return routes
}

func TestAdmissionControlCleanupMultipleFilters(t *testing.T) {
	for _, ti := range []struct {
		msg string
		doc string
	}{{
		msg: "no filter",
		doc: `* -> "%s"`,
	}, {
		msg: "one not matching filter",
		doc: `* -> setRequestHeader("Foo", "bar") -> "%s"`,
	}, {
		msg: "one matching filter",
		doc: `* -> admissionControl("app", "active", "1s", 5, 10, 0.1, 0.95, 0.5) -> "%s"`,
	}, {
		msg: "two matching filters",
		doc: `* -> admissionControl("app", "active", "1s", 5, 10, 0.1, 0.95, 0.5) -> admissionControl("app2", "active", "1s", 5, 10, 0.1, 0.95, 0.5) -> "%s"`,
	}, {
		msg: "multi filter with multiple matching filters",
		doc: `* -> setRequestHeader("Foo", "bar") -> admissionControl("app", "active", "1s", 5, 10, 0.1, 0.95, 0.5) -> status(200) -> admissionControl("app2", "active", "1s", 5, 10, 0.1, 0.95, 0.5) -> setRequestHeader("Foo2", "bar2") -> admissionControl("app3", "active", "1s", 5, 10, 0.1, 0.95, 0.5) -> setRequestHeader("Foo3", "bar3") -> "%s"`,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			ch := make(chan []*routing.Route)
			validationProcessor := &validationPostProcessorNumberOfFilters{
				ch: ch,
			}

			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			fspec := NewAdmissionControl(Options{})
			spec, ok := fspec.(*AdmissionControlSpec)
			if !ok {
				t.Fatal("FilterSpec is not a AdmissionControlSpec")
			}
			preProcessor := spec.PreProcessor()
			postProcessor := spec.PostProcessor()

			r := eskip.MustParse(fmt.Sprintf(ti.doc, backend.URL))

			fr := make(filters.Registry)
			fr.Register(fspec)
			fr.Register(builtin.NewSetRequestHeader())
			fr.Register(builtin.NewStatus())

			dc := testdataclient.New(r)
			defer dc.Close()

			proxy := proxytest.WithRoutingOptions(fr, routing.Options{
				DataClients:    []routing.DataClient{dc},
				PostProcessors: []routing.PostProcessor{postProcessor, validationProcessor},
				PreProcessors:  []routing.PreProcessor{preProcessor},
			})
			defer proxy.Close()

			result := <-ch
			if result == nil {
				t.Error("Failed to cleanup filters correctly, found more than one admissionControl filter in one route")
			}
		})
	}
}

type validationPostProcessorNumberOfFilters struct {
	ch chan []*routing.Route
}

// Do validates if number of admissionControl filters are less than
// one for each route passed, if so it returns routes as it is
// if not it returns nil.
func (vpp *validationPostProcessorNumberOfFilters) Do(routes []*routing.Route) []*routing.Route {
	i := 0
	for _, r := range routes {
		j := 0
		for _, f := range r.Filters {
			if _, ok := f.Filter.(*admissionControl); ok {
				j++
			}
		}
		i = max(i, j)
	}

	if i > 1 {
		go func() { vpp.ch <- nil }()
		return nil
	}
	go func() { vpp.ch <- routes }()
	return routes
}

func max(i, j int) int {
	if i > j {
		return i
	}
	return j
}
