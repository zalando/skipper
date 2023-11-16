package auth_test

import (
	"net/http"
	"testing"

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
