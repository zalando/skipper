package routing

import (
	"testing"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing/testdataclient"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/logging/loggingtest"
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

func TestLogging(t *testing.T) {
	const r1 = `
		r1_1: Path("/foo") -> "https://foo.example.org";
		r1_2: Path("/bar") -> "https://bar.example.org";
		r1_3: Path("/baz") -> "https://baz.example.org";
	`

	const r2 = `
		r2_1: Path("/qux") -> "https://qux.example.org";
		r2_2: Path("/quux") -> "https://quux.example.org";
	`

	const r3 = ""

	createClients := func() ([]DataClient, error) {
		dc1, err := testdataclient.NewDoc(r1)
		if err != nil {
			return nil, err
		}

		dc2, err := testdataclient.NewDoc(r2)
		if err != nil {
			return nil, err
		}

		dc3, err := testdataclient.NewDoc(r3)
		if err != nil {
			return nil, err
		}

		return []DataClient{dc1, dc2, dc3}, nil
	}

	init := func(l logging.Logger, clients []DataClient) *Routing {
		return New(Options{
			DataClients: clients,
			Log: l,
		})
	}

	testInitial := func(t *testing.T, logCount int) {
		c, err := createClients()
		if err != nil {
			t.Error(err)
			return
		}

		testLog := loggingtest.New()
		rt := init(testLog, c)
		defer rt.Close()

		if err := testLog.WaitForN("route settings", logCount, 120 * time.Millisecond); err != nil {
			t.Error(err)
		}
	}

	testUpdate := func(t *testing.T, logCountInit int, logCountUpdate int) {
		c, err := createClients()
		if err != nil {
			t.Error(err)
			return
		}

		testLog := loggingtest.New()
		rt := init(testLog, c)
		defer rt.Close()

		if err := testLog.WaitForN("route settings", logCountInit, 120 * time.Millisecond); err != nil {
			t.Error(err)
		}

		testLog.Reset()

		c[0].(*testdataclient.Client).UpdateDoc(
			`r1_1: Path("/foo_mod") -> "https://foo.example.org"`,
			[]string{"r1_2"},
		)
		c[1].(*testdataclient.Client).UpdateDoc(
			`r2_1: Path("/qux_mod") -> "https://qux.example.org"`,
			nil,
		)

		if err := testLog.WaitForN("route settings", logCountUpdate, 120 * time.Millisecond); err != nil {
			t.Error(err)
		}
	}

	t.Run("full", func(t *testing.T) {
		t.Run("initial", func(t *testing.T) {
			testInitial(t, 5)
		})

		t.Run("update", func(t *testing.T) {
			testUpdate(t, 5, 3)
		})
	})
}
