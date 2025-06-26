package net

import (
	"fmt"
	"net/http"
	"testing"
)

func TestHostPatch(t *testing.T) {
	for config, cases := range map[HostPatch]map[string]string{
		{}: {
			"":                  "",
			"example.org":       "example.org",
			"example.org.":      "example.org.",
			"example.org:8080":  "example.org:8080",
			"example.org.:8080": "example.org.:8080",
			"127.0.0.1":         "127.0.0.1",
			"127.0.0.1:9090":    "127.0.0.1:9090",
			"::1":               "::1",
			"[::1]:9090":        "[::1]:9090",
			"EXAMPLE.ORG":       "EXAMPLE.ORG",
			"EXAMPLE.ORG:8080":  "EXAMPLE.ORG:8080",
			"EXAMPLE.ORG.:8080": "EXAMPLE.ORG.:8080",
		},
		{RemovePort: true}: {
			"":                  "",
			"example.org":       "example.org",
			"example.org.":      "example.org.",
			"example.org:8080":  "example.org",
			"example.org.:8080": "example.org.",
			"127.0.0.1":         "127.0.0.1",
			"127.0.0.1:9090":    "127.0.0.1",
			"::1":               "::1",
			"[::1]:9090":        "::1",
			"EXAMPLE.ORG":       "EXAMPLE.ORG",
			"EXAMPLE.ORG:8080":  "EXAMPLE.ORG",
			"EXAMPLE.ORG.:8080": "EXAMPLE.ORG.",
		},
		{RemoveTrailingDot: true}: {
			"":                  "",
			"example.org":       "example.org",
			"example.org.":      "example.org",
			"example.org:8080":  "example.org:8080",
			"example.org.:8080": "example.org:8080",
			"EXAMPLE.ORG":       "EXAMPLE.ORG",
			"EXAMPLE.ORG:8080":  "EXAMPLE.ORG:8080",
			"EXAMPLE.ORG.:8080": "EXAMPLE.ORG:8080",
		},
		{RemovePort: true, RemoveTrailingDot: true}: {
			"":                  "",
			"example.org":       "example.org",
			"example.org.":      "example.org",
			"example.org:8080":  "example.org",
			"example.org.:8080": "example.org",
			"127.0.0.1":         "127.0.0.1",
			"127.0.0.1:9090":    "127.0.0.1",
			"::1":               "::1",
			"[::1]:9090":        "::1",
			"EXAMPLE.ORG":       "EXAMPLE.ORG",
			"EXAMPLE.ORG:8080":  "EXAMPLE.ORG",
			"EXAMPLE.ORG.:8080": "EXAMPLE.ORG",
		},
		{RemovePort: true, RemoveTrailingDot: true, ToLower: true}: {
			"":                  "",
			"example.org":       "example.org",
			"example.org.":      "example.org",
			"example.org:8080":  "example.org",
			"example.org.:8080": "example.org",
			"127.0.0.1":         "127.0.0.1",
			"127.0.0.1:9090":    "127.0.0.1",
			"::1":               "::1",
			"[::1]:9090":        "::1",
			"EXAMPLE.ORG":       "example.org",
			"EXAMPLE.ORG:8080":  "example.org",
			"EXAMPLE.ORG.:8080": "example.org",
		},
	} {
		for host, expected := range cases {
			t.Run(fmt.Sprintf("%s->%+v", host, config), func(t *testing.T) {
				got := config.Apply(host)
				if expected != got {
					t.Errorf("host mismatch, expected: %s, got: %s", expected, got)
				}
			})
		}
	}
}

func TestHostPatchHandler(t *testing.T) {
	host := ""
	rh := HostPatchHandler{
		HostPatch{
			RemovePort:        true,
			RemoveTrailingDot: true,
		},
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { host = r.Host }),
	}
	rh.ServeHTTP(nil, &http.Request{Host: "example.com.:8080"})

	if host != "example.com" {
		t.Errorf("expected example.com, got: %s", host)
	}
}

func BenchmarkHostPatchCommon(b *testing.B) {
	hp := HostPatch{RemovePort: true, RemoveTrailingDot: true, ToLower: true}
	if hp.Apply("www.example.org") != "www.example.org" {
		b.Fatal("expected www.example.org")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hp.Apply("www.example.org")
	}
}

func BenchmarkHostPatchUncommon(b *testing.B) {
	hp := HostPatch{RemovePort: true, RemoveTrailingDot: true, ToLower: true}
	if hp.Apply("WWW.EXAMPLE.ORG.:8080") != "www.example.org" {
		b.Fatal("expected www.example.org")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hp.Apply("WWW.EXAMPLE.ORG.:8080")
	}
}
