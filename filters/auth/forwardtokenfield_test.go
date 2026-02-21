package auth

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestForwardTokenFieldInvalidHeadername(t *testing.T) {
	ftSpec := NewForwardTokenField()
	filterArgs := []any{"test-%header\n", "aaa"}
	_, err := ftSpec.CreateFilter(filterArgs)
	if err == nil {
		t.Fatalf("bad header name")
	}
}

func TestForwardTokenFieldInvalidNumber(t *testing.T) {
	ftSpec := NewForwardTokenField()
	filterArgs := []any{"header1"}
	_, err := ftSpec.CreateFilter(filterArgs)
	if err == nil {
		t.Fatalf("invalid number of parameters")
	}
}

func TestForwardFieldField(t *testing.T) {
	spec := NewForwardTokenField()
	if spec.Name() != filters.ForwardTokenFieldName {
		t.Error("wrong name")
	}

	const headerName = "Header1"

	for _, ti := range []struct {
		msg           string
		path          string
		stateKey      string
		state         any
		expectedValue string
	}{{
		path:     "claims.key1",
		stateKey: oidcClaimsCacheKey,
		state: tokenContainer{
			Claims: map[string]any{
				"key1": "value1",
			}},
		expectedValue: "value1",
	}, {
		path:     "key1",
		stateKey: tokeninfoCacheKey,
		state: map[string]any{
			"key1": "value1",
		},
		expectedValue: "value1",
	}, {
		path:     "key1",
		stateKey: tokenintrospectionCacheKey,
		state: tokenIntrospectionInfo{
			"key1": "value1",
		},
		expectedValue: "value1",
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			var state = map[string]any{
				ti.stateKey: ti.state,
			}

			c := &filtertest.Context{FRequest: &http.Request{}, FStateBag: state}
			c.FRequest.Header = make(http.Header)

			c.FRequest.Header.Set(headerName, "oldValue")

			f, _ := spec.CreateFilter([]any{headerName, ti.path})
			f.Request(c)
			if c.FRequest.Header.Get(headerName) != ti.expectedValue {
				t.Fatalf("%s %s does not contain value %s", ti.stateKey, headerName, ti.expectedValue)
			}
		})
	}
}

func TestForwardFieldFieldEmpty(t *testing.T) {
	spec := NewForwardTokenField()
	if spec.Name() != filters.ForwardTokenFieldName {
		t.Error("wrong name")
	}

	f, err := spec.CreateFilter([]any{"Header1", "claims.key1"})
	if err != nil {
		t.Fatal("failed to create filter")
	}

	var state = map[string]any{
		oidcClaimsCacheKey: tokenContainer{},
	}

	c := &filtertest.Context{FRequest: &http.Request{}, FStateBag: state}
	c.FRequest.Header = make(http.Header)
	c.FRequest.Header.Set("Header1", "blbabla")

	f.Request(c)

	if c.FRequest.Header.Get("Header1") != "blbabla" {
		t.Fatalf("Header1 should not be overridden")
	}
}
