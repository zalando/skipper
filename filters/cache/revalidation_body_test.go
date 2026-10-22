package cache

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestProxy_CacheVary304PreservesBodyMetadata(t *testing.T) {
	for _, mode := range []string{"rfc-loopback", "force-loopback", "force-direct"} {
		for _, contentLength := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/representationLength=%t", mode, contentLength), func(t *testing.T) {
				testProxyCacheVary304(t, mode, contentLength)
			})
		}
	}
}

func testProxyCacheVary304(t *testing.T, mode string, contentLength bool) {
	payload := strings.Repeat("representation", 200)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Vary", "Accept-Language")
		w.Header().Set("ETag", `"stable"`)
		w.Header().Set("Cache-Control", "public, max-age=15")
		if r.Header.Get("If-None-Match") == `"stable"` {
			if contentLength {
				w.Header().Set("Content-Length", fmt.Sprint(len(r.Header.Get("Accept-Language")+payload)))
			}
			w.Header().Set("X-Revalidated", "yes")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		fmt.Fprint(w, r.Header.Get("Accept-Language")+payload)
	}))
	defer backend.Close()
	spec := NewCacheFilter(Options{MaxBytes: 8 << 20}).(*cacheSpec)
	defer spec.Close()
	registry := make(filters.Registry)
	registry.Register(spec)
	args := []any{"15s", "2s", "15s", "24h", "Authorization"}
	if mode == "rfc-loopback" {
		args = []any{"Authorization"}
	}
	proxy := proxytest.New(registry, &eskip.Route{
		Id: "conditional", Path: "/contentful", Backend: backend.URL,
		Filters: []*eskip.Filter{{Name: spec.Name(), Args: args}},
	})
	defer proxy.Close()
	client := proxy.Client()
	filter, err := spec.CreateFilter(args)
	if err != nil {
		t.Fatal(err)
	}
	f := filter.(*cacheFilter)
	f.listenAddr = strings.TrimPrefix(proxy.URL, "http://")
	for _, language := range []string{"en", "de"} {
		req, _ := http.NewRequest("GET", proxy.URL+"/contentful", nil)
		req.Header.Set("Authorization", "fixture")
		req.Header.Set("Accept-Language", language)
		for cycle := range 2 {
			rsp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(rsp.Body)
			rsp.Body.Close()
			if err != nil || string(body) != language+payload {
				t.Fatalf("cycle=%d language=%s bytes=%d ContentLength=%d err=%v", cycle, language, len(body), rsp.ContentLength, err)
			}
			if cycle == 1 && rsp.Header.Get("X-Revalidated") != "yes" {
				t.Fatal("304 metadata not merged")
			}
			if cycle == 0 {
				keyReq := req.Clone(context.Background())
				keyReq.URL.Scheme = ""
				base := cacheKey("conditional", keyReq, f.keyHeaders)
				key := varyKey(base, keyReq, []string{"Accept-Language"})
				before, _ := f.storage.GetSilent(context.Background(), key)
				if before == nil {
					t.Fatal("seed absent")
				}
				dispatch := ""
				if mode == "force-direct" {
					dispatch = backend.URL
				}
				f.doRevalidate(key, keyReq, dispatch, "", base)
				after, _ := f.storage.GetSilent(context.Background(), key)
				if after == nil || string(after.Payload) != string(before.Payload) {
					t.Error("304 changed stored payload")
				}
			}
		}
	}
}
