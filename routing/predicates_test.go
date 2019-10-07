package routing

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routing/testdataclient"
)

func TestPredicateList(t *testing.T) {
	type check struct {
		request    *http.Request
		expectedID string
		allowedIDs []string
	}

	for _, test := range []struct {
		title   string
		options MatchingOptions
		routes  []*eskip.Route
		checks  []check
	}{{

		title: "only legacy predicate",
		routes: []*eskip.Route{{
			Id: "test",
			Headers: map[string]string{
				"X-Test": "foo",
			},
			BackendType: eskip.ShuntBackend,
		}, {
			Id:          "catchAll",
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL: &url.URL{Path: "/"},
				Header: http.Header{
					"X-Test": []string{"foo"},
				},
			},
			expectedID: "test",
		}, {
			request: &http.Request{
				URL: &url.URL{Path: "/"},
			},
			expectedID: "catchAll",
		}},
	}, {

		title: "only legacy predicate, path",
		routes: []*eskip.Route{{
			Id:          "test",
			Path:        "/foo",
			BackendType: eskip.ShuntBackend,
		}, {
			Id:          "catchAll",
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL: &url.URL{Path: "/foo"},
			},
			expectedID: "test",
		}, {
			request: &http.Request{
				URL: &url.URL{Path: "/"},
			},
			expectedID: "catchAll",
		}},
	}, {

		title: "only predicate list",
		routes: []*eskip.Route{{
			Id: "test",
			Predicates: []*eskip.Predicate{{
				Name: "Header",
				Args: []interface{}{
					"X-Test",
					"foo",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id:          "catchAll",
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL: &url.URL{Path: "/"},
				Header: http.Header{
					"X-Test": []string{"foo"},
				},
			},
			expectedID: "test",
		}, {
			request: &http.Request{
				URL: &url.URL{Path: "/"},
			},
			expectedID: "catchAll",
		}},
	}, {

		title: "only predicate list, path",
		routes: []*eskip.Route{{
			Id: "test",
			Predicates: []*eskip.Predicate{{
				Name: "Path",
				Args: []interface{}{
					"/foo",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id:          "catchAll",
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL: &url.URL{Path: "/foo"},
			},
			expectedID: "test",
		}, {
			request: &http.Request{
				URL: &url.URL{Path: "/"},
			},
			expectedID: "catchAll",
		}},
	}, {

		title: "mixed, no conflict",
		routes: []*eskip.Route{{
			Id: "testLegacyAndList",
			Headers: map[string]string{
				"X-Test-Legacy": "foo",
			},
			Predicates: []*eskip.Predicate{{
				Name: "Header",
				Args: []interface{}{
					"X-Test-New",
					"foo",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "testLegacyOnly",
			Headers: map[string]string{
				"X-Test-Legacy": "foo",
			},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "testNewOnly",
			Predicates: []*eskip.Predicate{{
				Name: "Header",
				Args: []interface{}{
					"X-Test-New",
					"foo",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id:          "catchAll",
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL: &url.URL{Path: "/"},
				Header: http.Header{
					"X-Test-Legacy": []string{"foo"},
					"X-Test-New":    []string{"foo"},
				},
			},
			expectedID: "testLegacyAndList",
		}, {
			request: &http.Request{
				URL: &url.URL{Path: "/"},
				Header: http.Header{
					"X-Test-Legacy": []string{"foo"},
				},
			},
			expectedID: "testLegacyOnly",
		}, {
			request: &http.Request{
				URL: &url.URL{Path: "/"},
				Header: http.Header{
					"X-Test-New": []string{"foo"},
				},
			},
			expectedID: "testNewOnly",
		}, {
			request: &http.Request{
				URL: &url.URL{Path: "/"},
			},
			expectedID: "catchAll",
		}},
	}, {
		title: "mixed, with conflict",
		routes: []*eskip.Route{{
			Id: "testLegacyAndList",
			Headers: map[string]string{
				"X-Test": "foo",
			},
			Predicates: []*eskip.Predicate{{
				Name: "Header",
				Args: []interface{}{
					"X-Test",
					"bar",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "testLegacyOnly",
			Headers: map[string]string{
				"X-Test": "foo",
			},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "testNewOnly",
			Predicates: []*eskip.Predicate{{
				Name: "Header",
				Args: []interface{}{
					"X-Test",
					"bar",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id:          "catchAll",
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL: &url.URL{Path: "/"},
				Header: http.Header{
					"X-Test": []string{"foo", "bar"},
				},
			},
			allowedIDs: []string{"testLegacyAndList", "testLegacyOnly", "testNewOnly"},
		}, {
			request: &http.Request{
				URL: &url.URL{Path: "/"},
				Header: http.Header{
					"X-Test": []string{"foo"},
				},
			},
			allowedIDs: []string{"testLegacyAndList", "testLegacyOnly"},
		}, {
			request: &http.Request{
				URL: &url.URL{Path: "/"},
				Header: http.Header{
					"X-Test": []string{"bar"},
				},
			},
			allowedIDs: []string{"testLegacyAndList", "testNewOnly"},
		}, {
			request: &http.Request{
				URL: &url.URL{Path: "/"},
			},
			expectedID: "catchAll",
		}},
	}, {

		title: "mixed, path conflict",
		routes: []*eskip.Route{{
			Id:   "testLegacyAndList",
			Path: "/foo",
			Predicates: []*eskip.Predicate{{
				Name: "Path",
				Args: []interface{}{
					"/bar",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id:          "testLegacyOnly",
			Path:        "/foo",
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "testNewOnly",
			Predicates: []*eskip.Predicate{{
				Name: "Path",
				Args: []interface{}{
					"/bar",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id:          "catchAll",
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL: &url.URL{Path: "/foo"},
			},
			allowedIDs: []string{"testLegacyAndList", "testLegacyOnly"},
		}, {
			request: &http.Request{
				URL: &url.URL{Path: "/bar"},
			},
			allowedIDs: []string{"testLegacyAndList", "testNewOnly"},
		}, {
			request: &http.Request{
				URL: &url.URL{Path: "/"},
			},
			expectedID: "catchAll",
		}},
	}, {
		title: "path regexp with trailing slash",
		routes: []*eskip.Route{{
			Id: "foo",
			Predicates: []*eskip.Predicate{{
				Name: "PathRegexp",
				Args: []interface{}{"^/foo/bar/baz-[0-9-]+/$"},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL: &url.URL{Path: "/foo/bar/baz-42-0/"},
			},
			expectedID: "foo",
		}},
	}, {
		title:   "path regexp with trailing slash, ignore",
		options: IgnoreTrailingSlash,
		routes: []*eskip.Route{{
			Id: "foo",
			Predicates: []*eskip.Predicate{{
				Name: "PathRegexp",
				Args: []interface{}{"^/foo/bar/baz-[0-9-]+/$"},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL: &url.URL{Path: "/foo/bar/baz-42-0/"},
			},
			expectedID: "foo",
		}},
	}} {
		t.Run(test.title, func(t *testing.T) {
			dc := testdataclient.New(test.routes)

			l := loggingtest.New()
			l.Unmute()
			defer l.Close()

			rt := New(Options{
				DataClients:     []DataClient{dc},
				MatchingOptions: test.options,
				Log:             l,
			})
			defer rt.Close()

			l.WaitFor("route settings applied", 120*time.Millisecond)

			for _, check := range test.checks {
				checkTitle := check.expectedID
				if checkTitle == "" {
					checkTitle = fmt.Sprint(check.allowedIDs)
				}

				t.Run("expecting "+checkTitle, func(t *testing.T) {
					r, _ := rt.Route(check.request)
					if r == nil {
						t.Error("route not found")
						return
					}

					if check.expectedID != "" && r.Id != check.expectedID {
						t.Error("routing failed")
						t.Log(
							"wrong route matched:", r.Id,
							"but expected:", check.expectedID,
						)

						return
					}

					if check.expectedID != "" {
						t.Log("matched:", check.expectedID)
						return
					}

					for i := range check.allowedIDs {
						if r.Id == check.allowedIDs[i] {
							t.Log("matched:", check.allowedIDs[i])
							return
						}
					}

					t.Error("not allowed ID:", r.Id)
				})
			}
		})
	}
}
