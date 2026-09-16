package tls

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestProxySSLVerifyOffFilter(t *testing.T) {
	spec := NewProxySSLVerifyOff()

	t.Run("name", func(t *testing.T) {
		if got := spec.Name(); got != filters.ProxySSLVerifyOffName {
			t.Errorf("expected %q, got %q", filters.ProxySSLVerifyOffName, got)
		}
	})

	t.Run("create_no_args", func(t *testing.T) {
		f, err := spec.CreateFilter(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f == nil {
			t.Fatal("expected non-nil filter")
		}
	})

	t.Run("create_with_args_rejected", func(t *testing.T) {
		_, err := spec.CreateFilter([]any{"unexpected"})
		if err == nil {
			t.Fatal("expected error for unexpected argument, got nil")
		}
	})

	t.Run("request_sets_statebag_key", func(t *testing.T) {
		f, err := spec.CreateFilter(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ctx := &filtertest.Context{
			FRequest:  &http.Request{},
			FStateBag: make(map[string]any),
		}
		f.Request(ctx)
		val, ok := ctx.FStateBag[filters.BackendSkipTLSVerify]
		if !ok {
			t.Fatalf("expected %q key in StateBag, not found", filters.BackendSkipTLSVerify)
		}
		if val != true {
			t.Errorf("expected true, got %v", val)
		}
	})

	t.Run("response_is_noop", func(t *testing.T) {
		f, err := spec.CreateFilter(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ctx := &filtertest.Context{
			FRequest:  &http.Request{},
			FStateBag: make(map[string]any),
		}
		// Should not panic or set anything
		f.Response(ctx)
		if len(ctx.FStateBag) != 0 {
			t.Errorf("expected empty StateBag after Response, got %v", ctx.FStateBag)
		}
	})
}
