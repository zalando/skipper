package hedge_test

import (
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"github.com/zalando/skipper/filters/hedge"
)

func TestDisableHedgeFilter(t *testing.T) {
	spec := hedge.NewDisableHedge()

	if spec.Name() != filters.DisableHedgeName {
		t.Fatalf("unexpected name %q", spec.Name())
	}

	for _, tc := range []struct {
		name string
		args []any
		ok   bool
	}{
		{"no args", nil, true},
		{"unexpected arg", []any{"x"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := spec.CreateFilter(tc.args)
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v, want ok=%v", err, tc.ok)
			}
			if !tc.ok {
				return
			}

			ctx := &filtertest.Context{FStateBag: map[string]any{}}
			f.Request(ctx)
			if v, _ := ctx.StateBag()[filters.BackendHedgeDisabled].(bool); !v {
				t.Fatal("state bag flag not set")
			}
		})
	}
}

func TestShadowTrafficPreProcessor(t *testing.T) {
	for _, tc := range []struct {
		name     string
		route    string
		disabled bool
	}{
		{"plain route", `r: * -> "http://a.example.org";`, false},
		{"tee", `r: * -> tee("http://b.example.org") -> "http://a.example.org";`, true},
		{"deprecated Tee", `r: * -> Tee("http://b.example.org") -> "http://a.example.org";`, true},
		{"teenf", `r: * -> teenf("http://b.example.org") -> "http://a.example.org";`, true},
		{"teeLoopback", `r: * -> teeLoopback("k") -> "http://a.example.org";`, true},
		{"teeResponse", `r: * -> teeResponse("http://b.example.org") -> "http://a.example.org";`, true},
		{"Tee predicate", `r: Tee("k") -> "http://a.example.org";`, true},
		{"already disabled", `r: Tee("k") -> disableHedge() -> "http://a.example.org";`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			routes := hedge.NewShadowTrafficPreProcessor().Do(eskip.MustParse(tc.route))

			n := 0
			for _, f := range routes[0].Filters {
				if f.Name == filters.DisableHedgeName {
					n++
				}
			}
			if tc.disabled && n != 1 {
				t.Fatalf("got %d disableHedge filters, want exactly 1", n)
			}
			if !tc.disabled && n != 0 {
				t.Fatalf("got %d disableHedge filters, want 0", n)
			}
		})
	}
}
