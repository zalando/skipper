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

func TestEncodeRequestHeaderNameUknown(t *testing.T) {
	spec := encodeHeaderSpec{typ: -5}
	if spec.Name() != "unknown" {
		t.Fatalf("Failed to get unknown filter type, got: %q", spec.Name())
	}
}

func TestCreateFilterEncodeRequestHeader(t *testing.T) {
	for _, tt := range []struct {
		name    string
		sargs   []interface{}
		wantErr error
	}{
		{
			name:    "ISO8859_1",
			sargs:   []interface{}{"X-foo", "ISO8859_1"},
			wantErr: nil,
		},
		{
			name:    "ISO8859_10",
			sargs:   []interface{}{"X-foo", "ISO8859_10"},
			wantErr: nil,
		},
		{
			name:    "ISO8859_13",
			sargs:   []interface{}{"X-foo", "ISO8859_13"},
			wantErr: nil,
		},
		{
			name:    "ISO8859_14",
			sargs:   []interface{}{"X-foo", "ISO8859_14"},
			wantErr: nil,
		},
		{
			name:    "ISO8859_15",
			sargs:   []interface{}{"X-foo", "ISO8859_15"},
			wantErr: nil,
		},
		{
			name:    "ISO8859_16",
			sargs:   []interface{}{"X-foo", "ISO8859_16"},
			wantErr: nil,
		},
		{
			name:    "ISO8859_2",
			sargs:   []interface{}{"X-foo", "ISO8859_2"},
			wantErr: nil,
		},
		{
			name:    "ISO8859_3",
			sargs:   []interface{}{"X-foo", "ISO8859_3"},
			wantErr: nil,
		},
		{
			name:    "ISO8859_4",
			sargs:   []interface{}{"X-foo", "ISO8859_4"},
			wantErr: nil,
		},
		{
			name:    "ISO8859_5",
			sargs:   []interface{}{"X-foo", "ISO8859_5"},
			wantErr: nil,
		},
		{
			name:    "ISO8859_6",
			sargs:   []interface{}{"X-foo", "ISO8859_6"},
			wantErr: nil,
		},
		{
			name:    "ISO8859_7",
			sargs:   []interface{}{"X-foo", "ISO8859_7"},
			wantErr: nil,
		},
		{
			name:    "ISO8859_8",
			sargs:   []interface{}{"X-foo", "ISO8859_8"},
			wantErr: nil,
		},
		{
			name:    "ISO8859_9",
			sargs:   []interface{}{"X-foo", "ISO8859_9"},
			wantErr: nil,
		},
		{
			name:    "KOI8R",
			sargs:   []interface{}{"X-foo", "KOI8R"},
			wantErr: nil,
		},
		{
			name:    "KOI8U",
			sargs:   []interface{}{"X-foo", "KOI8U"},
			wantErr: nil,
		},
		{
			name:    "Macintosh",
			sargs:   []interface{}{"X-foo", "Macintosh"},
			wantErr: nil,
		},
		{
			name:    "MacintoshCyrillic",
			sargs:   []interface{}{"X-foo", "MacintoshCyrillic"},
			wantErr: nil,
		},
		{
			name:    "Windows1250",
			sargs:   []interface{}{"X-foo", "Windows1250"},
			wantErr: nil,
		},
		{
			name:    "Windows1251",
			sargs:   []interface{}{"X-foo", "Windows1251"},
			wantErr: nil,
		},
		{
			name:    "Windows1252",
			sargs:   []interface{}{"X-foo", "Windows1252"},
			wantErr: nil,
		},
		{
			name:    "Windows1253",
			sargs:   []interface{}{"X-foo", "Windows1253"},
			wantErr: nil,
		},
		{
			name:    "Windows1254",
			sargs:   []interface{}{"X-foo", "Windows1254"},
			wantErr: nil,
		},
		{
			name:    "Windows1255",
			sargs:   []interface{}{"X-foo", "Windows1255"},
			wantErr: nil,
		},
		{
			name:    "Windows1256",
			sargs:   []interface{}{"X-foo", "Windows1256"},
			wantErr: nil,
		},
		{
			name:    "Windows1257",
			sargs:   []interface{}{"X-foo", "Windows1257"},
			wantErr: nil,
		},
		{
			name:    "Windows1258",
			sargs:   []interface{}{"X-foo", "Windows1258"},
			wantErr: nil,
		},
		{
			name:    "Windows874",
			sargs:   []interface{}{"X-foo", "Windows874"},
			wantErr: nil,
		},
		{
			name:    "unknown",
			sargs:   []interface{}{"X-foo", "unknown"},
			wantErr: filters.ErrInvalidFilterParameters,
		},
		{
			name:    "error not enough arguments",
			sargs:   []interface{}{"X-foo"},
			wantErr: filters.ErrInvalidFilterParameters,
		},
		{
			name:    "type error key",
			sargs:   []interface{}{5, "X-foo"},
			wantErr: filters.ErrInvalidFilterParameters,
		},
		{
			name:    "type error value",
			sargs:   []interface{}{"X-foo", 5},
			wantErr: filters.ErrInvalidFilterParameters,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			args := make([]interface{}, 0)
			for _, v := range tt.sargs {
				if s, ok := v.(interface{}); ok {
					args = append(args, s)
				} else {
					t.Fatalf("Failed to convert %q to interface{}", v)
				}
			}

			for specT := range []encodeTyp{requestEncoder, responseEncoder} {
				spec := encodeHeaderSpec{typ: encodeTyp(specT)}
				f, err := spec.CreateFilter(args)
				if err != tt.wantErr {
					t.Fatalf("Failed to create filter with args %v want %v, got: %v", args, tt.wantErr, err)
				}
				if tt.wantErr == nil && f == nil {
					t.Fatal("Failed to get filter, got nil instead")
				}
			}
		})
	}

}

func Test_encodeRequestHeader(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		data string
		want []byte
	}{
		{
			name: "test request header Windows1252",
			doc:  `r: * -> encodeRequestHeader("Result", "Windows1252") -> logHeader("request")-> "%s";`,
			data: `für`,
			want: []byte{102, 252, 114}, //`f\xfcr`,
		}, {
			name: "test request header Windows1252 fail",
			doc:  `r: * -> encodeRequestHeader("Result", "Windows1252") -> logHeader("request")-> "%s";`,
			data: `f界r`,
			want: nil,
		}, {
			name: "test request header Windows1252 no data",
			doc:  `r: * -> encodeRequestHeader("Result", "Windows1252") -> logHeader("request")-> "%s";`,
			data: "",
			want: nil,
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
		}, {
			name: "test response header Windows1252 fail",
			doc:  `r: * -> encodeResponseHeader("Result", "Windows1252") -> setResponseHeader("Result", "%s") -> <shunt>;`,
			data: `f界r`,
			want: nil,
		}, {
			name: "test response header Windows1252 no data",
			doc:  `r: * -> encodeResponseHeader("Result", "Windows1252") -> setResponseHeader("Result", "%s") -> <shunt>;`,
			data: "",
			want: nil,
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
