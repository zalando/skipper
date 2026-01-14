package auth_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/auth"
	"github.com/zalando/skipper/filters/filtertest"
)

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

func TestSetRequestHeaderFromSecretInvalidArgs(t *testing.T) {
	spec := auth.NewSetRequestHeaderFromSecret(nil)
	for _, def := range []string{
		`setRequestHeaderFromSecret()`,
		`setRequestHeaderFromSecret("X-Secret")`,
		`setRequestHeaderFromSecret("X-Secret", 1)`,
		`setRequestHeaderFromSecret(1, "/my-secret")`,
		`setRequestHeaderFromSecret("X-Secret", "/my-secret", 1)`,
		`setRequestHeaderFromSecret("X-Secret", "/my-secret", "prefix", 1)`,
		`setRequestHeaderFromSecret("X-Secret", "/my-secret", "prefix", "suffix", "garbage")`,
	} {
		t.Run(def, func(t *testing.T) {
			ff := eskip.MustParseFilters(def)
			require.Len(t, ff, 1)

			_, err := spec.CreateFilter(ff[0].Args)
			assert.Error(t, err)
		})
	}
}

func TestSetRequestHeaderFromSecret(t *testing.T) {
	spec := auth.NewSetRequestHeaderFromSecret(&testSecretsReader{
		name:   "/my-secret",
		secret: "secret-value",
	})

	assert.Equal(t, "setRequestHeaderFromSecret", spec.Name())

	for _, tc := range []struct {
		def, header, value string
	}{
		{
			def:    `setRequestHeaderFromSecret("X-Secret", "/my-secret")`,
			header: "X-Secret",
			value:  "secret-value",
		},
		{
			def:    `setRequestHeaderFromSecret("X-Secret", "/my-secret", "foo-")`,
			header: "X-Secret",
			value:  "foo-secret-value",
		},
		{
			def:    `setRequestHeaderFromSecret("X-Secret", "/my-secret", "foo-", "-bar")`,
			header: "X-Secret",
			value:  "foo-secret-value-bar",
		},
		{
			def:    `setRequestHeaderFromSecret("X-Secret", "/does-not-exist")`,
			header: "X-Secret",
			value:  "",
		},
	} {
		t.Run(tc.def, func(t *testing.T) {
			ff := eskip.MustParseFilters(tc.def)
			require.Len(t, ff, 1)

			f, err := spec.CreateFilter(ff[0].Args)
			assert.NoError(t, err)

			ctx := &filtertest.Context{
				FRequest: &http.Request{
					Header: http.Header{},
				},
			}
			f.Request(ctx)

			if tc.value != "" {
				assert.Equal(t, tc.value, ctx.FRequest.Header.Get(tc.header))
			} else {
				assert.NotContains(t, ctx.FRequest.Header, tc.header)
			}
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

func TestSetRequestHeaderFromSecretLogsErrorOnMissingSecret(t *testing.T) {
	out := captureLog(t, log.ErrorLevel)

	spec := auth.NewSetRequestHeaderFromSecret(&testSecretsReader{
		name:   "/my-secret",
		secret: "secret-value",
	})

	ff := eskip.MustParseFilters(`setRequestHeaderFromSecret("X-Secret", "/nonexistent-secret")`)
	require.Len(t, ff, 1)

	f, err := spec.CreateFilter(ff[0].Args)
	assert.NoError(t, err)

	ctx := &filtertest.Context{
		FRequest: &http.Request{
			Header: http.Header{},
		},
	}
	f.Request(ctx)

	logOutput := out.String()
	if !strings.Contains(logOutput, "/nonexistent-secret") || !strings.Contains(logOutput, "not found for setRequestHeaderFromSecret filter") {
		t.Errorf("expected log message about missing secret, got: %s", logOutput)
	}

	// Verify no header was set
	assert.NotContains(t, ctx.FRequest.Header, "X-Secret")
}
