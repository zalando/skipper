package routing_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/logging"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/predicates/primitive"
	"github.com/zalando/skipper/predicates/query"
	"github.com/zalando/skipper/routing"
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
			defer dc.Close()

			defs, err := dc.LoadAll()
			if err != nil {
				if !ti.err {
					t.Error(ti.routes, err)
				}

				return
			}

			erred := false
			o := &routing.Options{
				FilterRegistry: make(filters.Registry),
			}
			pr := make(map[string]routing.PredicateSpec)
			for _, d := range defs {
				if _, err := routing.ExportProcessRouteDef(o, pr, d); err != nil {
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

func TestProcessRouteDefErrors(t *testing.T) {
	for _, ti := range []struct {
		routes string
		err    string
	}{
		{
			`* -> True() -> <shunt>`,
			`trying to use "True" as filter, but it is only available as predicate`,
		}, {
			`* -> PathRegexp("/test") -> <shunt>`,
			`trying to use "PathRegexp" as filter, but it is only available as predicate`,
		}, {
			`* -> Unknown("/test") -> <shunt>`,
			`filter "Unknown" not found`,
		}, {
			`Unknown()  ->  <shunt>`,
			`predicate "Unknown" not found`,
		}, {
			`QueryParam() -> <shunt>`,
			`failed to create predicate "QueryParam": invalid predicate parameters`,
		}, {
			`* -> setPath() -> <shunt>`,
			`failed to create filter "setPath": invalid filter parameters`,
		},
	} {
		func() {
			dc, err := testdataclient.NewDoc(ti.routes)
			if err != nil {
				t.Error(ti.routes, err)

				return
			}
			defer dc.Close()

			defs, err := dc.LoadAll()
			if err != nil {
				t.Error(ti.routes, err)

				return
			}

			pr := map[string]routing.PredicateSpec{}
			for _, s := range []routing.PredicateSpec{primitive.NewTrue(), query.New()} {
				pr[s.Name()] = s
			}
			fr := make(filters.Registry)
			fr.Register(builtin.NewSetPath())
			o := &routing.Options{
				FilterRegistry: fr,
			}
			for _, d := range defs {
				_, err := routing.ExportProcessRouteDef(o, pr, d)
				if err == nil || err.Error() != ti.err {
					t.Errorf("expected error '%s'. Got: '%s'", ti.err, err)
				}
			}
		}()
	}
}

func TestProcessRouteDefWeight(t *testing.T) {
	cpm := map[string]routing.PredicateSpec{
		"WeightedPredicate10":      weightedPredicateSpec{name: "WeightedPredicate10", weight: 10},
		"WeightedPredicateMinus10": weightedPredicateSpec{name: "WeightedPredicateMinus10", weight: -10},
	}

	for _, ti := range []struct {
		route  string
		weight int
	}{
		{
			`Path("/foo") -> <shunt>`,
			0,
		}, {
			`WeightedPredicate10() -> <shunt>`,
			10,
		}, {
			`Weight(20) -> <shunt>`,
			20,
		}, {
			`Weight(20) && Weight(10)-> <shunt>`,
			30,
		}, {
			`WeightedPredicate10() && Weight(20) -> <shunt>`,
			30,
		}, {
			`WeightedPredicateMinus10() -> <shunt>`,
			-10,
		}, {
			`WeightedPredicateMinus10() && Weight(10) -> <shunt>`,
			0,
		}, {
			`WeightedPredicateMinus10() && Weight(20) -> <shunt>`,
			10,
		},
	} {
		func() {

			dc, err := testdataclient.NewDoc(ti.route)
			if err != nil {
				t.Error(ti.route, err)

				return
			}
			defer dc.Close()

			defs, err := dc.LoadAll()
			if err != nil {
				t.Error(ti.route, err)

				return
			}

			r := defs[0]

			_, weight, err := routing.ExportProcessPredicates(cpm, r.Predicates)
			if err != nil {
				t.Error(ti.route, err)

				return
			}

			if weight != ti.weight {
				t.Errorf("expected weight '%d'. Got: '%d' (%s)", ti.weight, weight, ti.route)

				return
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

	init := func(l logging.Logger, client routing.DataClient, suppress bool) *routing.Routing {
		return routing.New(routing.Options{
			DataClients:  []routing.DataClient{client},
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
		defer client.Close()

		testLog := loggingtest.New()
		defer testLog.Close()

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

func TestMetrics(t *testing.T) {
	t.Run("create filter latency", func(t *testing.T) {
		client, err := testdataclient.NewDoc(`
			r0: * -> slowCreate("100ms") -> slowCreate("200ms") -> slowCreate("100ms") -> <shunt>;
		`)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()

		metrics := &metricstest.MockMetrics{
			Now: time.Now(),
		}
		fr := make(filters.Registry)
		fr.Register(slowCreateSpec{})

		r := routing.New(routing.Options{
			DataClients:     []routing.DataClient{client},
			FilterRegistry:  fr,
			Metrics:         metrics,
			SignalFirstLoad: true,
		})
		defer r.Close()
		<-r.FirstLoad()

		metrics.WithMeasures(func(m map[string][]time.Duration) {
			assert.InEpsilonSlice(t, []time.Duration{
				100 * time.Millisecond,
				200 * time.Millisecond,
				100 * time.Millisecond,
			}, m["filter.slowCreate.create"], 0.1)
		})
	})
}

type weightedPredicateSpec struct {
	name   string
	weight int
}
type weightedPredicate struct{}

func (w weightedPredicate) Match(request *http.Request) bool {
	return true
}

func (w weightedPredicateSpec) Name() string {
	return w.name
}

func (w weightedPredicateSpec) Create([]interface{}) (routing.Predicate, error) {
	return weightedPredicate{}, nil
}

func (w weightedPredicateSpec) Weight() int {
	return w.weight
}

type (
	slowCreateSpec   struct{}
	slowCreateFilter struct{}
)

func (s slowCreateSpec) Name() string { return "slowCreate" }

func (s slowCreateSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	d, _ := time.ParseDuration(args[0].(string))

	time.Sleep(d)

	return slowCreateFilter{}, nil
}

func (s slowCreateFilter) Request(ctx filters.FilterContext) {}
func (s slowCreateFilter) Response(filters.FilterContext)    {}
