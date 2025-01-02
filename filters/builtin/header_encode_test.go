package builtin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/diag"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

func Test_encodeRequestHeader(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		data string
		want []byte
	}{
		{
			name: "test request header Windows1252",
			doc:  `r: * -> encodeRequestHeader("X-Test", "Windows1252") -> logHeader("request")-> "%s";`,
			data: `für`,
			want: []byte{102, 252, 114}, //`f\xfcr`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Result", r.Header.Get("Result"))
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			r := eskip.MustParse(fmt.Sprintf(tt.doc, backend.URL))
			fr := make(filters.Registry)
			fr.Register(NewEncodeRequestHeader())
			fr.Register(diag.NewLogHeader())

			dc := testdataclient.New(r)
			defer dc.Close()

			proxy := proxytest.WithRoutingOptions(fr, routing.Options{
				DataClients: []routing.DataClient{dc},
			})
			defer proxy.Close()

			req, err := http.NewRequest("GET", proxy.URL, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Result", tt.data)

			rsp, err := proxy.Client().Do(req)
			if err != nil {
				t.Fatalf("Failed to do request: %v", err)
			}
			defer rsp.Body.Close()
			if result := rsp.Header.Get("Result"); result != string(tt.want) {
				t.Fatalf("Failed to get %q, got %q", tt.want, result)
			}
		})
	}
}

func Test_encodeResponseHeader(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		data string
		want []byte
	}{
		{
			name: "test response header Windows1252",
			doc:  `r: * -> encodeResponseHeader("Result", "Windows1252") -> setResponseHeader("Result", "%s") -> <shunt>;`,
			data: `für`,
			want: []byte{102, 252, 114}, //`f\xfcr`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := eskip.MustParse(fmt.Sprintf(tt.doc, tt.data))
			fr := make(filters.Registry)
			fr.Register(NewEncodeResponseHeader())
			fr.Register(NewSetResponseHeader())

			dc := testdataclient.New(r)
			defer dc.Close()

			proxy := proxytest.WithRoutingOptions(fr, routing.Options{
				DataClients: []routing.DataClient{dc},
			})
			defer proxy.Close()

			req, err := http.NewRequest("GET", proxy.URL, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			rsp, err := proxy.Client().Do(req)
			if err != nil {
				t.Fatalf("Failed to do request: %v", err)
			}
			defer rsp.Body.Close()
			if result := rsp.Header.Get("Result"); result != string(tt.want) {
				t.Fatalf("Failed to get %q, got %q", tt.want, result)
			}

		})
	}
}
