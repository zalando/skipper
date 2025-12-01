package apiusagemonitoring

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_parseJwtBody_NoHeader(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "", nil)
	assert.NoError(t, err)
	req.Header = http.Header{}

	body := parseJwtBody(req)
	assert.Nil(t, body)
}

func Test_parseJwtBody_HeadersButNoAuthorization(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "", nil)
	assert.NoError(t, err)
	req.Header = http.Header{
		"foo": []string{"first_foo", "second_foo", "that_was_enough_foo"},
	}

	body := parseJwtBody(req)
	assert.Nil(t, body)
}

func Test_parseJwtBody_AuthorizationHeaderEmpty(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "", nil)
	assert.NoError(t, err)
	req.Header = http.Header{
		authorizationHeaderName: []string{""},
	}

	body := parseJwtBody(req)
	assert.Nil(t, body)
}

func Test_parseJwtBody_AuthorizationHeaderNotValidJwt(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "", nil)
	assert.NoError(t, err)
	req.Header = http.Header{
		authorizationHeaderName: []string{"Bearer foo.bar"},
	}

	body := parseJwtBody(req)
	assert.Nil(t, body)
}

func Test_parseJwtBody_AuthorizationHeader3PartsNotBase64Encoded(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "", nil)
	assert.NoError(t, err)
	req.Header = http.Header{
		authorizationHeaderName: []string{"Bearer foo.bar.moo"},
	}

	body := parseJwtBody(req)
	assert.Nil(t, body)
}

func Test_parseJwtBody_AuthorizationHeader3PartsBase64EncodedNotJson(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "", nil)
	assert.NoError(t, err)
	req.Header = http.Header{
		authorizationHeaderName: []string{"Bearer Zm9v.YmFy.bW9v"},
	}

	body := parseJwtBody(req)
	assert.Nil(t, body)
}

func Test_parseJwtBody_AuthorizationHeaderWithValidJwtBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "", nil)
	assert.NoError(t, err)
	req.Header = http.Header{
		authorizationHeaderName: []string{"Bearer Zm9v.eyJmb28iOiJiYXIifQ.bW9v"},
	}

	body := parseJwtBody(req)
	assert.Equal(t, jwtTokenPayload{"foo": "bar"}, body)
}

func BenchmarkParseJwtBody(b *testing.B) {
	req, err := http.NewRequest(http.MethodGet, "", nil)
	assert.NoError(b, err)
	req.Header = http.Header{
		authorizationHeaderName: []string{"Bearer Zm9v.eyJmb28iOiJiYXIifQ.bW9v"},
	}

	for b.Loop() {
		body := parseJwtBody(req)
		assert.Equal(b, jwtTokenPayload{"foo": "bar"}, body)
	}
}
