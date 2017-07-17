package routing

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper/eskip"
)

func TestSlice(t *testing.T) {
	tests := []struct {
		message string
		r       []*eskip.Route
		offset  int
		limit   int
		expect  []*eskip.Route
	}{
		{
			"empty routes",
			[]*eskip.Route{},
			0,
			0,
			[]*eskip.Route{},
		},
		{
			"to big offset routes",
			[]*eskip.Route{},
			1,
			0,
			[]*eskip.Route{},
		},
		{
			"to big offset and limit routes",
			[]*eskip.Route{},
			1,
			1,
			[]*eskip.Route{},
		},
	}

	for _, ti := range tests {
		res := slice(ti.r, ti.offset, ti.limit)
		if !cmp.Equal(res, ti.expect) {
			t.Fatalf("Failed test case '%s', got %v and expected %v", ti.message, res, ti.expect)
		}
	}
}
