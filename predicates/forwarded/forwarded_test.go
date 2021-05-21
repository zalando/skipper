package forwarded

import (
	"net/http"
	"net/url"
	"testing"
)

type request struct {
	url     string
	headers http.Header
}

func TestForwardedHost(t *testing.T) {
	testCases := []struct {
		msg     string
		host    string
		r       request
		matches bool
		isError bool
	}{{
		msg:     "Empty host should fail",
		host:    "",
		r:       request{},
		matches: false,
		isError: true,
	}, {
		msg:     "Empty Forwarded Header should not match",
		host:    "example.com",
		r:       request{},
		matches: false,
		isError: false,
	}, {
		msg:  "Same forwarded Header should match",
		host: "^example.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"for=192.0.2.60;proto=http;by=203.0.113.43;host=example.com"},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "Same forwarded Header should match",
		host: "^example.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"for=192.0.2.60;proto=http;host=example.com;by=203.0.113.43"},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "Same forwarded Header should match",
		host: "^example.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"host=example.com;for=192.0.2.60;proto=http;by=203.0.113.43"},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "Forwarded Header should match subdomains",
		host: "example.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"host=subdomain.example.com;for=192.0.2.60;proto=http;by=203.0.113.43"},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "Different forwarded Header should not match",
		host: "^example.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"for=192.0.2.60;proto=http;by=203.0.113.43;host=example.comma.org"},
			},
		},
		matches: false,
		isError: false,
	}, {
		msg:  "Different forwarded Header should not match",
		host: "example.com",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"for=192.0.2.60;proto=http;by=203.0.113.43;host=example.org"},
			},
		},
		matches: false,
		isError: false,
	}, {
		msg:  "Forwarded Header with no host should not match",
		host: "example.com",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"for=192.0.2.60;proto=http;by=203.0.113.43"},
			},
		},
		matches: false,
		isError: false,
	}, {
		msg:  "Forwarded Header only host should match",
		host: "example.com",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"host=example.com"},
			},
		},
		matches: true,
		isError: false,
	}}

	for _, tc := range testCases {

		t.Run(tc.msg, func(t *testing.T) {
			spec := NewForwardedHost()

			p, err := spec.Create([]interface{}{tc.host})
			hasError := err != nil
			if hasError || tc.isError {
				if !tc.isError {
					t.Fatal("Predicate creation failed")
				}

				if !hasError {
					t.Fatal("Predicate should have failed")
				}

				if hasError && tc.isError {
					return
				}
			}

			r, err := newRequest(tc.r)
			if err != nil {
				t.Fatal("Request creation failed")
			}

			m := p.Match(r)
			if m != tc.matches {
				t.Fatalf("Unexpected predicate match result: %t instead of %t", m, tc.matches)
			}
		})
	}
}

func TestForwardedProto(t *testing.T) {
	testCases := []struct {
		msg     string
		proto   string
		r       request
		matches bool
		isError bool
	}{{
		msg:     "Invalid protocol should error",
		proto:   "foobar",
		r:       request{},
		matches: false,
		isError: true,
	}, {
		msg:     "Empty protocol should error",
		proto:   "",
		r:       request{},
		matches: false,
		isError: true,
	}, {
		msg:     "Empty Forwarded Header should not match",
		proto:   "http",
		r:       request{},
		matches: false,
		isError: false,
	}, {
		msg:     "Empty Forwarded Header should not match",
		proto:   "https",
		r:       request{},
		matches: false,
		isError: false,
	}, {
		msg:   "Same forwarded Header should match",
		proto: "http",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"for=192.0.2.60;proto=http;by=203.0.113.43;host=example.com"},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:   "Same forwarded Header should match",
		proto: "https",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"for=192.0.2.60;proto=https;by=203.0.113.43;host=example.com"},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:   "Different forwarded Header should not match",
		proto: "https",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"for=192.0.2.60;proto=http;by=203.0.113.43;host=example.org"},
			},
		},
		matches: false,
		isError: false,
	}, {
		msg:   "Different forwarded Header should not match",
		proto: "http",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"for=192.0.2.60;proto=https;by=203.0.113.43;host=example.org"},
			},
		},
		matches: false,
		isError: false,
	}, {
		msg:   "Forwarded Header with no proto should not match",
		proto: "http",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"for=192.0.2.60;host=example.org;by=203.0.113.43"},
			},
		},
		matches: false,
		isError: false,
	}, {
		msg:   "Forwarded Header with no proto should not match",
		proto: "https",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"for=192.0.2.60;host=example.org;by=203.0.113.43"},
			},
		},
		matches: false,
		isError: false,
	}, {
		msg:   "Forwarded Header only proto should match",
		proto: "http",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"proto=http"},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:   "Forwarded Header only proto should match",
		proto: "https",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"proto=https"},
			},
		},
		matches: true,
		isError: false,
	}}

	for _, tc := range testCases {

		t.Run(tc.msg, func(t *testing.T) {
			spec := NewForwardedProto()

			p, err := spec.Create([]interface{}{tc.proto})
			hasError := err != nil
			if hasError || tc.isError {
				if !tc.isError {
					t.Fatal("Predicate creation failed")
				}

				if !hasError {
					t.Fatal("Predicate should have failed")
				}

				if hasError && tc.isError {
					return
				}
			}

			r, err := newRequest(tc.r)
			if err != nil {
				t.Fatal("Request creation failed")
			}

			m := p.Match(r)
			if m != tc.matches {
				t.Fatalf("Unexpected predicate match result: %t instead of %t", m, tc.matches)
			}
		})
	}
}

func newRequest(r request) (*http.Request, error) {
	u, err := url.Parse(r.url)

	if err != nil {
		return nil, err
	}

	return &http.Request{
		URL:    u,
		Header: r.headers,
	}, nil
}
