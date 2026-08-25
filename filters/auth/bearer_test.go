package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/secrets"
)

func Test_bearerInjectorSpec_Name(t *testing.T) {
	b := &bearerInjectorSpec{}
	if got := b.Name(); got != filters.BearerInjectorName {
		t.Errorf("bearerInjectorSpec.Name() = %v, want %v", got, filters.BearerInjectorName)
	}
}

func Test_bearerInjectorSpec_CreateFilter(t *testing.T) {
	tests := []struct {
		name    string
		args    []any
		want    *bearerInjectorFilter
		wantErr bool
	}{
		{
			name:    "no arg",
			wantErr: true,
		},
		{
			name:    "too many args",
			args:    []any{"foo", "bar"},
			wantErr: true,
		},
		{
			name:    "wrong args",
			args:    []any{3},
			wantErr: true,
		},
		{
			name:    "a secretname",
			args:    []any{"foo"},
			want:    &bearerInjectorFilter{secretName: "foo"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &bearerInjectorSpec{}
			got, err := b.CreateFilter(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("bearerInjectorSpec.CreateFilter() error = %v, wantErr %v", err, tt.wantErr)
			} else if err == nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("bearerInjectorSpec.CreateFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

type testSecretsReader struct {
	name   string
	secret string
}

func (tsr *testSecretsReader) GetSecret(name string) ([]byte, bool) {
	if name == tsr.name {
		return []byte(tsr.secret), true
	}
	return nil, false
}

func (*testSecretsReader) Close() {}

func Test_bearerInjectorFilter_Request(t *testing.T) {
	goodtoken := "goodtoken"
	goodsecret := "goodsecret"
	tests := []struct {
		name          string
		secretName    string
		secretsReader secrets.SecretsReader
		want          int
	}{
		{
			name:       "Test the happy path ",
			secretName: goodsecret,
			secretsReader: &testSecretsReader{
				name:   goodsecret,
				secret: goodtoken,
			},
			want: http.StatusOK,
		},
		{
			name:       "Test the wrong secretname ",
			secretName: "wrongname",
			secretsReader: &testSecretsReader{
				name:   goodsecret,
				secret: goodtoken,
			},
			want: http.StatusUnauthorized,
		},
		{
			name:       "Test the wrong token ",
			secretName: goodsecret,
			secretsReader: &testSecretsReader{
				name:   goodsecret,
				secret: "wrongtoken",
			},
			want: http.StatusUnauthorized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got := r.Header.Get(authHeaderName)
				if authHeaderPrefix+goodtoken != got {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer backendServer.Close()

			var routeFilters []*eskip.Filter
			fr := make(filters.Registry)

			spec := NewBearerInjector(tt.secretsReader)
			filterArgs := []any{tt.secretName}
			_, err := spec.CreateFilter(filterArgs)
			if err != nil {
				t.Fatalf("error in creating filter")
			}
			fr.Register(spec)

			routeFilters = append(routeFilters, &eskip.Filter{Name: spec.Name(), Args: filterArgs})
			r := &eskip.Route{Filters: routeFilters, Backend: backendServer.URL}

			proxy := proxytest.New(fr, r)
			defer proxy.Close()

			reqURL, err := url.Parse(proxy.URL)
			if err != nil {
				t.Errorf("Failed to parse url %s: %v", proxy.URL, err)
			}
			req, err := http.NewRequest("GET", reqURL.String(), nil)
			if err != nil {
				t.Error(err)
				return
			}

			rsp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
			}
			if rsp.StatusCode != tt.want {
				t.Errorf("injection did not work as expected: got %d, want %d", rsp.StatusCode, tt.want)
			}
			rsp.Body.Close()
		})
	}
}

func captureLog(t *testing.T, level log.Level) *bytes.Buffer {
	oldOut := log.StandardLogger().Out
	var out bytes.Buffer
	log.SetOutput(&out)

	oldLevel := log.GetLevel()
	log.SetLevel(level)

	t.Cleanup(func() {
		log.SetOutput(oldOut)
		log.SetLevel(oldLevel)
	})
	return &out
}

func Test_bearerInjectorFilter_LogsErrorOnMissingSecret(t *testing.T) {
	out := captureLog(t, log.ErrorLevel)

	secretsReader := &testSecretsReader{
		name:   "existing-secret",
		secret: "token",
	}

	f := newBearerInjectorFilter("nonexistent-secret", secretsReader)

	ctx := &filtertest.Context{
		FRequest: &http.Request{
			Header: http.Header{},
		},
	}

	f.Request(ctx)

	logOutput := out.String()
	if !strings.Contains(logOutput, "nonexistent-secret") || !strings.Contains(logOutput, "not found for bearerinjector filter") {
		t.Errorf("expected log message about missing secret, got: %s", logOutput)
	}

	// Verify no header was set
	if ctx.FRequest.Header.Get(authHeaderName) != "" {
		t.Error("expected no Authorization header to be set when secret is missing")
	}
}
