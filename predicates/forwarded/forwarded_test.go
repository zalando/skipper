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
		host:    "^example\\.com$",
		r:       request{},
		matches: false,
		isError: false,
	}, {
		msg:  "Same forwarded non-quoted Header should match",
		host: "^example\\.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{`for=192.0.2.60;proto=http;by=203.0.113.43;host=example.com`},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "Same forwarded Header should match",
		host: "^example\\.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{`for=192.0.2.60;proto=http;by=203.0.113.43;host="example.com"`},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "Same forwarded Header should match",
		host: "^example\\.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{`for=192.0.2.60;proto=http;host="example.com";by=203.0.113.43`},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "Same forwarded Header should match",
		host: "^example\\.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{`host="example.com";for=192.0.2.60;proto=http;by=203.0.113.43`},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "Forwarded Header should match subdomains",
		host: "example\\.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{`host="subdomain.example.com";for=192.0.2.60;proto=http;by=203.0.113.43`},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "Different forwarded Header should not match",
		host: "^example\\.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{`for=192.0.2.60;proto=http;by=203.0.113.43;host="example.comma.org"`},
			},
		},
		matches: false,
		isError: false,
	}, {
		msg:  "Different forwarded Header should not match",
		host: "^example\\.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{`for=192.0.2.60;proto=http;by=203.0.113.43;host="example.org"`},
			},
		},
		matches: false,
		isError: false,
	}, {
		msg:  "Forwarded Header with no host should not match",
		host: "^example\\.com$",
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
		host: "^example\\.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{`host="example.com"`},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "In multiple forwarded header only host should match",
		host: "^example\\.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{`host="example.com",host="example.com"`},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "In multiple forwarded header only host with spaces should match",
		host: "^example\\.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{` host="example.com", host="example.com"`},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "In multiple forwarded header complete should match",
		host: "^example\\.com$",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{`for=2b12:c7f:565d:b000:d4a5:ed98:1eea:d290;by=2b12:26f0:4000:2ae::3751,by=10.0.0.60;host=example.com;proto=https, for=10.0.0.60;by=10.0.0.61,by=10.0.0.62;host=example.com;proto=https`},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "wildcard host should match",
		host: "^([a-z0-9]+((-[a-z0-9]+)?)*[.]example[.]org[.]?(:[0-9]+)?)$", // *.example.org
		r: request{
			url: "https://test.example.org/index.html",
			headers: http.Header{
				"Forwarded": []string{`host="test.example.org"`},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "wildcard 2 host should match",
		host: "^([a-z0-9]+((-[a-z0-9]+)?)*[.]example[.]org[.]?(:[0-9]+)?)$", // *.example.org
		r: request{
			url: "https://test-v2.example.org/index.html",
			headers: http.Header{
				"Forwarded": []string{`host="test-v2.example.org"`},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "wildcard 3 host should match",
		host: "^([a-z0-9]+((-[a-z0-9]+)?)*[.]example[.]org[.]?(:[0-9]+)?)$", // *.example.org
		r: request{
			url: "https://test-v2-v3.example.org/index.html",
			headers: http.Header{
				"Forwarded": []string{`host="test-v2-v3.example.org"`},
			},
		},
		matches: true,
		isError: false,
	}, {
		msg:  "wildcard 4 host shouldn't match",
		host: "^([a-z0-9]+((-[a-z0-9]+)?)*[.]example[.]org[.]?(:[0-9]+)?)$", // *.example.org
		r: request{
			url: "https://test-.example.org/index.html",
			headers: http.Header{
				"Forwarded": []string{`host="test-.example.org"`},
			},
		},
		matches: false,
		isError: false,
	}}

	for _, tc := range testCases {

		t.Run(tc.msg, func(t *testing.T) {
			spec := NewForwardedHost()

			p, err := spec.Create([]interface{}{tc.host})
			hasError := err != nil
			if hasError || tc.isError {
				if !tc.isError {
					t.Fatalf("Predicate creation failed, %s", err)
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
		msg:   "Incorrect protocol should not match forwarded header",
		proto: "HTTP",
		r: request{
			url: "https://myproxy.com/index.html",
			headers: http.Header{
				"Forwarded": []string{"for=192.0.2.60;proto=http;by=203.0.113.43;host=example.org"},
			},
		},
		matches: false,
		isError: true,
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

func TestForwardedDocumentationExamples(t *testing.T) {

	header := http.Header{
		"Forwarded": []string{"host=example.com;proto=https, host=example.org"},
	}

	testCases := []struct {
		msg     string
		host    string
		proto   string
		r       request
		matches bool
	}{{
		msg:  "First host does not match",
		host: "^example\\.com$",
		r: request{
			url:     "https://myproxy.com/index.html",
			headers: header,
		},
		matches: false,
	}, {
		msg:  "Last host matches",
		host: "^example\\.org$",
		r: request{
			url:     "https://myproxy.com/index.html",
			headers: header,
		},
		matches: true,
	}, {
		msg:   "Last host and last proto match",
		host:  "^example\\.org$",
		proto: "https",
		r: request{
			url:     "https://myproxy.com/index.html",
			headers: header,
		},
		matches: true,
	}, {
		msg:   "First forwarded host and proto do not match",
		host:  "^example\\.com$",
		proto: "https",
		r: request{
			url:     "https://myproxy.com/index.html",
			headers: header,
		},
		matches: false,
	}}

	for _, tc := range testCases {

		t.Run(tc.msg, func(t *testing.T) {

			m := true

			if tc.proto != "" {
				protoSpec := NewForwardedProto()

				p, _ := protoSpec.Create([]interface{}{tc.proto})

				r, err := newRequest(tc.r)
				if err != nil {
					t.Fatal("Request creation failed")
				}

				m = m && p.Match(r)
			}

			if tc.host != "" {
				hostSpec := NewForwardedHost()

				p, _ := hostSpec.Create([]interface{}{tc.host})

				r, err := newRequest(tc.r)
				if err != nil {
					t.Fatal("Request creation failed")
				}

				m = m && p.Match(r)
			}

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
