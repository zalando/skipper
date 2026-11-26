package proxy_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
)

func BenchmarkHedgeDisabled(b *testing.B) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	route := eskip.MustParse(fmt.Sprintf("r: * -> %q;", backend.URL))
	testProxy := proxytest.New(builtin.MakeRegistry(), route[0])
	defer testProxy.Close()
	client := testProxy.Client()
	request, err := http.NewRequest(http.MethodGet, testProxy.URL, nil)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		response, err := client.Do(request)
		if err != nil {
			b.Fatal(err)
		}
		response.Body.Close()
	}
}
