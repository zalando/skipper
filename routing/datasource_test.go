package routing

import (
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing/testdataclient"
)

func TestNoMultipleTreePredicates(t *testing.T) {
	for _, ti := range []struct {
		routes string
		err    bool
	}{{
		`Path("/foo") && Path("/bar") -> <shunt>`,
		true,
	}, {
		`Path("/foo") && PathSubtree("/bar") -> <shunt>`,
		true,
	}, {
		`PathSubtree("/foo") && PathSubtree("/bar") -> <shunt>`,
		true,
	}, {
		`Path("/foo") -> <shunt>`,
		false,
	}, {
		`PathSubtree("/foo") -> <shunt>`,
		false,
	}} {
		func() {
			dc, err := testdataclient.NewDoc(ti.routes)
			if err != nil {
				if !ti.err {
					t.Error(ti.routes, err)
				}

				return
			}

			defs, err := dc.LoadAll()
			if err != nil {
				if !ti.err {
					t.Error(ti.routes, err)
				}

				return
			}

			erred := false
			pr := make(map[string]PredicateSpec)
			fr := make(filters.Registry)
			for _, d := range defs {
				if _, err := processRouteDef(pr, fr, d); err != nil {
					erred = true
					break
				}
			}

			if erred != ti.err {
				t.Error("unexpected error result", erred, ti.err)
			}
		}()
	}
}
