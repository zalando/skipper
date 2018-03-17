package routing

import (
	"testing"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/logging/loggingtest"
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

func TestLogging(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	const routes = `
		r1_1: Path("/foo") -> "https://foo.example.org";
		r1_2: Path("/bar") -> "https://bar.example.org";
		r1_3: Path("/baz") -> "https://baz.example.org";
		r1_4: Path("/qux") -> "https://qux.example.org";
		r1_5: Path("/quux") -> "https://quux.example.org";
	`

	init := func(l logging.Logger, client DataClient, suppress bool) *Routing {
		return New(Options{
			DataClients:  []DataClient{client},
			Log:          l,
			SuppressLogs: suppress,
		})
	}

	testUpdate := func(
		t *testing.T, suppress bool,
		initQuery string, initCount int,
		upsertQuery string, upsertCount int,
		deleteQuery string, deleteCount int,
	) {
		client, err := testdataclient.NewDoc(routes)
		if err != nil {
			t.Error(err)
			return
		}

		testLog := loggingtest.New()
		defer testLog.Close()
		// testLog.Unmute()

		rt := init(testLog, client, suppress)
		defer rt.Close()

		if err := testLog.WaitFor("route settings applied", 120*time.Millisecond); err != nil {
			t.Error(err)
			return
		}

		count := testLog.Count(initQuery)
		if count != initCount {
			t.Error("unexpected count of log entries", count)
			t.Log("expected", initCount, initQuery)
			t.Log("got     ", count)
			return
		}

		testLog.Reset()

		client.UpdateDoc(
			`r1_1: Path("/foo_mod") -> "https://foo.example.org";
			r1_4: Path("/qux_mod") -> "https://qux.example.org"`,
			[]string{"r1_2"},
		)

		if err := testLog.WaitFor("route settings applied", 120*time.Millisecond); err != nil {
			t.Error(err)
			return
		}

		count = testLog.Count(upsertQuery)
		if count != upsertCount {
			t.Error("unexpected count of log entries", count)
			return
		}

		count = testLog.Count(deleteQuery)
		if count != deleteCount {
			t.Error("unexpected count of log entries", count)
			return
		}
	}

	t.Run("full", func(t *testing.T) {
		testUpdate(
			t, false,
			"route settings, reset", 5,
			"route settings, update, route:", 2,
			"route settings, update, deleted", 1,
		)
	})

	t.Run("suppressed", func(t *testing.T) {
		testUpdate(
			t, true,
			"route settings, reset", 2,
			"route settings, update, upsert count:", 1,
			"route settings, update, delete count:", 1,
		)
	})
}
