package routing

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routing/testdataclient"
)

func TestPredicateList(t *testing.T) {
	type check struct {
		request        *http.Request
		expectedID     string
		allowedIDs     []string
		expectedParams map[string]string
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
				Args: []any{
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
				Args: []any{
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

		title: "path with multiple wildcards",
		routes: []*eskip.Route{{
			Id: "one",
			Predicates: []*eskip.Predicate{{
				Name: "Path",
				Args: []any{
					"/foo/:one/:two",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL: &url.URL{Path: "/foo/x/y"},
			},
			expectedID: "one",
			expectedParams: map[string]string{
				"one": "x",
				"two": "y",
			},
		}},
	}, {

		title: "path wildcard conflict",
		routes: []*eskip.Route{{
			Id: "one",
			Predicates: []*eskip.Predicate{{
				Name: "Path",
				Args: []any{
					"/foo/:one",
				},
			}, {
				Name: "Header",
				Args: []any{
					"X-Test",
					"one",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "two",
			Predicates: []*eskip.Predicate{{
				Name: "Path",
				Args: []any{
					"/foo/:two",
				},
			}, {
				Name: "Header",
				Args: []any{
					"X-Test",
					"two",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL: &url.URL{Path: "/foo/x"},
				Header: http.Header{
					"X-Test": []string{"one"},
				},
			},
			expectedID: "one",
			expectedParams: map[string]string{
				"one": "x",
			},
		}, {
			request: &http.Request{
				URL: &url.URL{Path: "/foo/x"},
				Header: http.Header{
					"X-Test": []string{"two"},
				},
			},
			expectedID: "two",
			expectedParams: map[string]string{
				"two": "x",
			},
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
				Args: []any{
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
				Args: []any{
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
				Args: []any{
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
				Args: []any{
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
				Args: []any{
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
				Args: []any{
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
		title: "path wildcard and path subtree",
		routes: []*eskip.Route{{
			Id: "star",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{"^foo[.]example[.]org$"},
			}, {
				Name: "Path",
				Args: []any{"/*p1"},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "subtree",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{"^bar[.]example[.]org$"},
			}, {
				Name: "PathSubtree",
				Args: []any{"/"},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL:  &url.URL{Path: "/baz"},
				Host: "foo.example.org",
			},
			expectedID: "star",
			expectedParams: map[string]string{
				"p1": "/baz",
			},
		}, {
			// no match when trailing slash not ignored
			request: &http.Request{
				URL:  &url.URL{Path: "/"},
				Host: "foo.example.org",
			},
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/qux"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/qux/"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/qux/quz"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}},
	}, {
		title: "path and path subtree conflict",
		routes: []*eskip.Route{{
			Id: "path",
			Predicates: []*eskip.Predicate{{
				Name: "Path",
				Args: []any{"/foo"},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "pathSubtree",
			Predicates: []*eskip.Predicate{{
				Name: "Method",
				Args: []any{"GET"},
			}, {
				Name: "PathSubtree",
				Args: []any{"/foo"},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL:    &url.URL{Path: "/foo"},
				Method: "POST",
			},
			expectedID: "path",
		}, {
			request: &http.Request{
				URL:    &url.URL{Path: "/foo"},
				Method: "GET",
			},
			expectedID: "pathSubtree",
		}},
	}, {
		title: "path and path subtree conflict, path more specific",
		routes: []*eskip.Route{{
			Id: "path",
			Predicates: []*eskip.Predicate{{
				Name: "Method",
				Args: []any{"GET"},
			}, {
				Name: "Path",
				Args: []any{"/foo"},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "pathSubtree",
			Predicates: []*eskip.Predicate{{
				Name: "PathSubtree",
				Args: []any{"/foo"},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL:    &url.URL{Path: "/foo"},
				Method: "GET",
			},
			expectedID: "path",
		}, {
			request: &http.Request{
				URL:    &url.URL{Path: "/foo/bar"},
				Method: "GET",
			},
			expectedID: "pathSubtree",
		}},
	}, {
		title:   "path wildcard and path subtree, ignore trailing slash",
		options: IgnoreTrailingSlash,
		routes: []*eskip.Route{{
			Id: "star",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{"^foo[.]example[.]org$"},
			}, {
				Name: "Path",
				Args: []any{"/*p1"},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "subtree",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{"^bar[.]example[.]org$"},
			}, {
				Name: "PathSubtree",
				Args: []any{"/"},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL:  &url.URL{Path: "/baz"},
				Host: "foo.example.org",
			},
			expectedID:     "star",
			expectedParams: map[string]string{"p1": "/baz"},
		}, {
			// no match when trailing slash not ignored
			request: &http.Request{
				URL:  &url.URL{Path: "/"},
				Host: "foo.example.org",
			},
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/qux"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/qux/"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/qux/quz"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}},
	}, {
		title: "path wildcard and path subtree, non-root",
		routes: []*eskip.Route{{
			Id: "star",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{"^foo[.]example[.]org$"},
			}, {
				Name: "Path",
				Args: []any{"/api/*p1"},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "subtree",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{"^bar[.]example[.]org$"},
			}, {
				Name: "PathSubtree",
				Args: []any{"/api"},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL:  &url.URL{Path: "/api/baz"},
				Host: "foo.example.org",
			},
			expectedID: "star",
			expectedParams: map[string]string{
				"p1": "/baz",
			},
		}, {
			// no match when trailing slash not ignored
			request: &http.Request{
				URL:  &url.URL{Path: "/api"},
				Host: "foo.example.org",
			},
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/api"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/api/qux"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/api/qux/"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/api/qux/quz"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}},
	}, {
		title:   "path wildcard and path subtree, ignore trailing slash",
		options: IgnoreTrailingSlash,
		routes: []*eskip.Route{{
			Id: "star",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{"^foo[.]example[.]org$"},
			}, {
				Name: "Path",
				Args: []any{"/api/*p1"},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "subtree",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{"^bar[.]example[.]org$"},
			}, {
				Name: "PathSubtree",
				Args: []any{"/api"},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL:  &url.URL{Path: "/api/baz"},
				Host: "foo.example.org",
			},
			expectedID:     "star",
			expectedParams: map[string]string{"p1": "/baz"},
		}, {
			// no match when trailing slash not ignored
			request: &http.Request{
				URL:  &url.URL{Path: "/api"},
				Host: "foo.example.org",
			},
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/api"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/api/qux"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/api/qux/"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/api/qux/quz"},
				Host: "bar.example.org",
			},
			expectedID: "subtree",
		}},
	}, {
		title: "path subtree, and path, both with free wildcard params",
		routes: []*eskip.Route{{
			Id: "star",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^foo.example.org$",
				},
			}, {
				Name: "Path",
				Args: []any{
					"/api/*p1",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "subtree",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^bar.example.org$",
				},
			}, {
				Name: "PathSubtree",
				Args: []any{
					"/api/*p2",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL:  &url.URL{Path: "/api/baz"},
				Host: "foo.example.org",
			},
			expectedID:     "star",
			expectedParams: map[string]string{"p1": "/baz"},
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/api/baz"},
				Host: "bar.example.org",
			},
			expectedID:     "subtree",
			expectedParams: map[string]string{"p2": "/baz"},
		}},
	}, {
		title: "path subtree, and path, only path with free wildcard param",
		routes: []*eskip.Route{{
			Id: "star",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^foo.example.org$",
				},
			}, {
				Name: "Path",
				Args: []any{
					"/api/*p1",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "subtree",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^bar.example.org$",
				},
			}, {
				Name: "PathSubtree",
				Args: []any{
					"/api",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL:  &url.URL{Path: "/api/baz"},
				Host: "foo.example.org",
			},
			expectedID:     "star",
			expectedParams: map[string]string{"p1": "/baz"},
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/api/baz"},
				Host: "bar.example.org",
			},
			expectedID:     "subtree",
			expectedParams: map[string]string{"*": "/baz"},
		}},
	}, {
		title: "path subtree, and path, path with unnamed, subtree with named free wildcard param",
		routes: []*eskip.Route{{
			Id: "star",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^foo.example.org$",
				},
			}, {
				Name: "Path",
				Args: []any{
					"/api/**",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "subtree",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^bar.example.org$",
				},
			}, {
				Name: "PathSubtree",
				Args: []any{
					"/api/*p2",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL:  &url.URL{Path: "/api/baz"},
				Host: "foo.example.org",
			},
			expectedID:     "star",
			expectedParams: map[string]string{"*": "/baz"},
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/api/baz"},
				Host: "bar.example.org",
			},
			expectedID:     "subtree",
			expectedParams: map[string]string{"p2": "/baz"},
		}},
	}, {
		title: "path subtree, and path, path with named, subtree with unnamed free wildcard param",
		routes: []*eskip.Route{{
			Id: "star",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^foo.example.org$",
				},
			}, {
				Name: "Path",
				Args: []any{
					"/api/*p1",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "subtree",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^bar.example.org$",
				},
			}, {
				Name: "PathSubtree",
				Args: []any{
					"/api/**",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL:  &url.URL{Path: "/api/baz"},
				Host: "foo.example.org",
			},
			expectedID:     "star",
			expectedParams: map[string]string{"p1": "/baz"},
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/api/baz"},
				Host: "bar.example.org",
			},
			expectedID:     "subtree",
			expectedParams: map[string]string{"*": "/baz"},
		}},
	}, {
		title: "path subtree, and path, both with free wildcard params, in root",
		routes: []*eskip.Route{{
			Id: "star",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^foo.example.org$",
				},
			}, {
				Name: "Path",
				Args: []any{
					"/*p1",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "subtree",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^bar.example.org$",
				},
			}, {
				Name: "PathSubtree",
				Args: []any{
					"/*p2",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL:  &url.URL{Path: "/baz"},
				Host: "foo.example.org",
			},
			expectedID:     "star",
			expectedParams: map[string]string{"p1": "/baz"},
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/baz"},
				Host: "bar.example.org",
			},
			expectedID:     "subtree",
			expectedParams: map[string]string{"p2": "/baz"},
		}},
	}, {
		title: "path subtree, and path, only path with free wildcard param, in root",
		routes: []*eskip.Route{{
			Id: "star",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^foo.example.org$",
				},
			}, {
				Name: "Path",
				Args: []any{
					"/*p1",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "subtree",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^bar.example.org$",
				},
			}, {
				Name: "PathSubtree",
				Args: []any{
					"/",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL:  &url.URL{Path: "/baz"},
				Host: "foo.example.org",
			},
			expectedID:     "star",
			expectedParams: map[string]string{"p1": "/baz"},
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/baz"},
				Host: "bar.example.org",
			},
			expectedID:     "subtree",
			expectedParams: map[string]string{"*": "/baz"},
		}},
	}, {
		title: "path subtree, and path, path with unnamed, subtree with named free wildcard param, in root",
		routes: []*eskip.Route{{
			Id: "star",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^foo.example.org$",
				},
			}, {
				Name: "Path",
				Args: []any{
					"/**",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "subtree",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^bar.example.org$",
				},
			}, {
				Name: "PathSubtree",
				Args: []any{
					"/*p2",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL:  &url.URL{Path: "/baz"},
				Host: "foo.example.org",
			},
			expectedID:     "star",
			expectedParams: map[string]string{"*": "/baz"},
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/baz"},
				Host: "bar.example.org",
			},
			expectedID:     "subtree",
			expectedParams: map[string]string{"p2": "/baz"},
		}},
	}, {
		title: "path subtree, and path, path with named, subtree with unnamed free wildcard param, in root",
		routes: []*eskip.Route{{
			Id: "star",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^foo.example.org$",
				},
			}, {
				Name: "Path",
				Args: []any{
					"/*p1",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}, {
			Id: "subtree",
			Predicates: []*eskip.Predicate{{
				Name: "Host",
				Args: []any{
					"^bar.example.org$",
				},
			}, {
				Name: "PathSubtree",
				Args: []any{
					"/**",
				},
			}},
			BackendType: eskip.ShuntBackend,
		}},
		checks: []check{{
			request: &http.Request{
				URL:  &url.URL{Path: "/baz"},
				Host: "foo.example.org",
			},
			expectedID:     "star",
			expectedParams: map[string]string{"p1": "/baz"},
		}, {
			request: &http.Request{
				URL:  &url.URL{Path: "/baz"},
				Host: "bar.example.org",
			},
			expectedID:     "subtree",
			expectedParams: map[string]string{"*": "/baz"},
		}},
	}, {
		title: "path regexp with trailing slash",
		routes: []*eskip.Route{{
			Id: "foo",
			Predicates: []*eskip.Predicate{{
				Name: "PathRegexp",
				Args: []any{"^/foo/bar/baz-[0-9-]+/$"},
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
				Args: []any{"^/foo/bar/baz-[0-9-]+/$"},
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
			defer dc.Close()

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
					r, p := rt.Route(check.request)
					if check.expectedID == "" && len(check.allowedIDs) == 0 {
						if r != nil {
							t.Error("unexpected route match")
						}

						return
					}

					if r == nil {
						t.Error("route not found")
						return
					}

					if check.expectedID != "" && r.Id != check.expectedID {
						t.Errorf(
							"routing failed; matched route: %s, expected: %s",
							r.Id,
							check.expectedID,
						)

						return
					}

					if check.expectedID == "" {
						var found bool
						if slices.Contains(check.allowedIDs, r.Id) {
							found = true
						}

						if !found {
							t.Error("not allowed ID:", r.Id)
							return
						}
					}

					if check.expectedParams != nil {
						if len(p) != len(check.expectedParams) {
							t.Errorf(
								"unexpected count of params; expected: %d, got: %d",
								len(check.expectedParams),
								len(p),
							)

							return
						}

						for k, v := range check.expectedParams {
							if p[k] != v {
								t.Errorf(
									"unexpected value for param: %s=%s",
									k,
									p[k],
								)

								return
							}
						}
					}
				})
			}
		})
	}
}
