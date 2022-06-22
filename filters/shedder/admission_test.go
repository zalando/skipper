package shedder

import (
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/net"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

func TestAdmissionControl(t *testing.T) {
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

			spec := NewAdmissionControl(Options{})
			args := make([]interface{}, 0, 6)
			args = append(args, "testmetric", ti.mode, ti.d.String(), ti.windowsize, ti.minRequests, ti.successThreshold, ti.maxrejectprobability, ti.exponent)
			_, err := spec.CreateFilter(args)
			if err != nil {
				t.Logf("args: %+v", args...)
				t.Fatalf("error creating filter: %v", err)
				return
			}

			fr := make(filters.Registry)
			fr.Register(spec)
			r := &eskip.Route{Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}}, Backend: backend.URL}

			proxy := proxytest.New(fr, r)
			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Fatalf("Failed to parse url %s: %v", proxy.URL, err)
			}

			client := net.NewClient(net.Options{})
			req, err := http.NewRequest("GET", reqURL.String(), nil)
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

func TestAdmissionControlCleanup(t *testing.T) {
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
			// spec := NewAdmissionControl(Options{})
			// postProcessor, ok := spec.(routing.PostProcessor)
			// if !ok {
			// 	t.Fatal("AdmissionControl is not a PostProcessor")
			// }
			// args := make([]interface{}, 0, 6)
			// args = append(args, "testmetric", ti.mode, "10ms", 5, 1, 0.1, 0.95, 0.5)
			// _, err := spec.CreateFilter(args)
			// if err != nil {
			// 	t.Fatalf("error creating filter: %v", err)
			// 	return
			// }

			// fr := make(filters.Registry)
			// fr.Register(spec)
			// r := &eskip.Route{
			// 	Id:      "r",
			// 	Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}},
			// }
			// r.BackendType = eskip.ShuntBackend

			// acs, ok := spec.(*admissionControlSpec)
			// if ok {
			// 	acs.mu.Lock()

			// 	deleteIDs := []string{}

			// 	for _, id := range deleteIDs {
			// 		if ac, ok := acs.filters[id]; ok {
			// 			if !ac.closed {
			// 			}
			// 		}
			// 	}
			// 	acs.mu.Unlock()
			// }
			//postProcessor.Do([]*routing.Route{r})

			backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			fspec := NewAdmissionControl(Options{})
			spec, ok := fspec.(*AdmissionControlSpec)
			if !ok {
				t.Fatal("FilterSpec is not a AdmissionControlSpec")
			}
			preProcessor := spec.PreProcessor()
			postProcessor := spec.PostProcessor()

			args := make([]interface{}, 0, 6)
			args = append(args, "testmetric", ti.mode, "10ms", 5, 1, 0.1, 0.95, 0.5)
			_, err := spec.CreateFilter(args)
			if err != nil {
				t.Fatalf("error creating filter: %v", err)
				return
			}

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
			proxy := proxytest.WithRoutingOptions(fr, routing.Options{
				DataClients:    []routing.DataClient{dc},
				PostProcessors: []routing.PostProcessor{postProcessor},
				PreProcessors:  []routing.PreProcessor{preProcessor},
			}, r1)
			defer proxy.Close()

			dc.Update([]*eskip.Route{r1}, nil)

			deletedIDs := []string{r1.Id}
			dc.Update([]*eskip.Route{r2}, deletedIDs)
			time.Sleep(time.Second)
			postProcessor.mu.Lock()
			for _, id := range deletedIDs {
				if ac, ok := postProcessor.filters[id]; ok {
					if !ac.closed {
						t.Errorf("filter should be closed routeID: %s", id)
					}
				}
			}
			postProcessor.mu.Unlock()

			// preProcessor will only apply r2 (last wins)
			dc.Update([]*eskip.Route{r1, r2}, nil)

			deletedIDs = []string{r2.Id}
			dc.Update([]*eskip.Route{}, deletedIDs)
			time.Sleep(time.Second)
			postProcessor.mu.Lock()
			for _, id := range deletedIDs {
				if ac, ok := postProcessor.filters[id]; ok {
					if !ac.closed {
						t.Errorf("filter should be closed routeID: %s", id)
					}
				}
			}
			postProcessor.mu.Unlock()

		})
	}
}
