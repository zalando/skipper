package fadein

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/routing"
	"github.com/zalando/skipper/routing/testdataclient"
)

type createTestItem struct {
	name   string
	args   []any
	expect any
	fail   bool
}

func (test createTestItem) run(
	t *testing.T,
	init func() filters.Spec,
	box func(filters.Filter) any,
) {
	f, err := init().CreateFilter(test.args)
	if test.fail {
		if err == nil {
			t.Fatal("Failed to fail.")
		}

		return
	}

	if err != nil {
		t.Fatal(err)
	}

	if box(f) != test.expect {
		t.Fatalf("Unexpected value, expected: %v, got: %v.", test.expect, box(f))
	}
}

func TestCreateFadeIn(t *testing.T) {
	for _, test := range []createTestItem{{
		name: "no args",
		fail: true,
	}, {
		name: "too many args",
		args: []any{1, 2, 3},
		fail: true,
	}, {
		name: "wrong duration string",
		args: []any{"foo"},
		fail: true,
	}, {
		name: "wrong exponent type",
		args: []any{"3m", "foo"},
		fail: true,
	}, {
		name:   "duration as int",
		args:   []any{1000},
		expect: fadeIn{duration: time.Second, exponent: 1},
	}, {
		name:   "duration as float",
		args:   []any{float64(1000)},
		expect: fadeIn{duration: time.Second, exponent: 1},
	}, {
		name:   "duration as string",
		args:   []any{"1s"},
		expect: fadeIn{duration: time.Second, exponent: 1},
	}, {
		name:   "duration as time.Duration",
		args:   []any{time.Second},
		expect: fadeIn{duration: time.Second, exponent: 1},
	}, {
		name:   "exponent as int",
		args:   []any{"3m", 2},
		expect: fadeIn{duration: 3 * time.Minute, exponent: 2},
	}, {
		name:   "exponent as float",
		args:   []any{"3m", 2.0},
		expect: fadeIn{duration: 3 * time.Minute, exponent: 2},
	}} {
		t.Run(test.name, func(t *testing.T) {
			test.run(
				t,
				NewFadeIn,
				func(f filters.Filter) any { return f.(fadeIn) },
			)
		})
	}
}

func TestCreateEndpointCreated(t *testing.T) {
	now := time.Now()

	nows := func() string {
		b, err := now.MarshalText()
		if err != nil {
			t.Fatal(err)
		}

		return string(b)
	}

	// ensure same precision:
	now, err := time.Parse(time.RFC3339, nows())
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []createTestItem{{
		name: "no args",
		fail: true,
	}, {
		name: "few args",
		args: []any{"http://10.0.0.1:8080"},
		fail: true,
	}, {
		name: "too many args",
		args: []any{"http://10.0.0.1:8080", now, "foo"},
		fail: true,
	}, {
		name: "address not string",
		args: []any{42, now},
		fail: true,
	}, {
		name: "address not url",
		args: []any{string(rune(' ' - 1)), now},
		fail: true,
	}, {
		name: "invalid host",
		args: []any{"http://::1", now},
		fail: true,
	}, {
		name: "invalid time string",
		args: []any{"http://10.0.0.1:8080", "foo"},
		fail: true,
	}, {
		name: "invalid time type",
		args: []any{"http://10.0.0.1:8080", struct{}{}},
		fail: true,
	}, {
		name:   "future time",
		args:   []any{"http://10.0.0.1:8080", now.Add(time.Hour)},
		expect: endpointCreated{which: "http://10.0.0.1:8080", when: time.Time{}},
	}, {
		name:   "auto 80",
		args:   []any{"http://10.0.0.1", now},
		expect: endpointCreated{which: "http://10.0.0.1:80", when: now},
	}, {
		name:   "auto 443",
		args:   []any{"https://10.0.0.1", now},
		expect: endpointCreated{which: "https://10.0.0.1:443", when: now},
	}, {
		name:   "time as int",
		args:   []any{"http://10.0.0.1:8080", 42},
		expect: endpointCreated{which: "http://10.0.0.1:8080", when: time.Unix(42, 0)},
	}, {
		name:   "time as float",
		args:   []any{"http://10.0.0.1:8080", 42.0},
		expect: endpointCreated{which: "http://10.0.0.1:8080", when: time.Unix(42, 0)},
	}, {
		name:   "time as string",
		args:   []any{"http://10.0.0.1:8080", nows()},
		expect: endpointCreated{which: "http://10.0.0.1:8080", when: now},
	}, {
		name:   "time as time.Time",
		args:   []any{"http://10.0.0.1:8080", now},
		expect: endpointCreated{which: "http://10.0.0.1:8080", when: now},
	}} {
		t.Run(test.name, func(t *testing.T) {
			test.run(
				t,
				NewEndpointCreated,
				func(f filters.Filter) any { return f.(endpointCreated) },
			)
		})
	}
}

func TestPostProcessor(t *testing.T) {
	createRouting := func(t *testing.T, routes string, endpointRegistry *routing.EndpointRegistry) (*routing.Routing, func(string)) {
		dc, err := testdataclient.NewDoc(routes)
		if err != nil {
			t.Fatal(err)
		}

		rt := routing.New(routing.Options{
			DataClients: []routing.DataClient{dc},
			FilterRegistry: filters.Registry{
				filters.FadeInName:          NewFadeIn(),
				filters.EndpointCreatedName: NewEndpointCreated(),
			},
			PostProcessors: []routing.PostProcessor{
				loadbalancer.NewAlgorithmProvider(),
				endpointRegistry,
				NewPostProcessor(PostProcessorOptions{EndpointRegistry: endpointRegistry}),
			},
			SignalFirstLoad: true,
		})
		<-rt.FirstLoad()
		return rt, func(nextDoc string) {
			if err := dc.UpdateDoc(nextDoc, nil); err != nil {
				t.Fatal(err)
			}

			// lazy way of making sure that the routes were processed:
			time.Sleep(9 * time.Millisecond)
		}
	}

	route := func(rt *routing.Routing, path string) *routing.Route {
		r, _ := rt.Route(&http.Request{URL: &url.URL{Path: path}})
		return r
	}

	nows := func(t *testing.T) string {
		now := time.Now()
		b, err := now.MarshalText()
		if err != nil {
			t.Fatal(err)
		}

		return string(b)
	}

	t.Run("post-process LB route with fade-in", func(t *testing.T) {
		const routes = `
			foo: Path("/foo") -> "https://www.example.org";
			bar: Path("/bar") -> fadeIn("1m") -> <"http://10.0.0.1:8080", "http://10.0.0.2:8080">;
			baz: Path("/baz") -> <"http://10.0.1.1:8080", "http://10.0.1.2:8080">
		`

		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		defer endpointRegistry.Close()
		rt, _ := createRouting(t, routes, endpointRegistry)

		foo := route(rt, "/foo")
		if foo == nil || foo.LBFadeInDuration != 0 {
			t.Fatal("failed to preserve non-LB route")
		}

		bar := route(rt, "/bar")
		if bar == nil || bar.LBFadeInDuration != time.Minute {
			t.Fatal("failed to postprocess LB route")
		}

		for _, ep := range bar.LBEndpoints {
			if endpointRegistry.GetMetrics(ep.Host).DetectedTime().IsZero() {
				t.Fatal("failed to set detection time")
			}
		}

		baz := route(rt, "/baz")
		if baz == nil || baz.LBFadeInDuration != 0 {
			t.Fatal("failed to preserve non-fade LB route")
		}
	})

	t.Run("invalid endpoint address", func(t *testing.T) {
		const routes = `
			* -> fadeIn("1m") -> <"http://::">
		`

		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		defer endpointRegistry.Close()
		rt, _ := createRouting(t, routes, endpointRegistry)
		r := route(rt, "/")
		if r != nil {
			t.Fatal("created invalid LB endpoint")
		}
	})

	t.Run("negative duration", func(t *testing.T) {
		const routes = `
			* -> fadeIn("-1m") -> <"http://10.0.0.1:8080">
		`

		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		defer endpointRegistry.Close()
		rt, _ := createRouting(t, routes, endpointRegistry)
		r := route(rt, "/")
		if r == nil || len(r.LBEndpoints) == 0 {
			t.Fatal("failed to ignore negative duration")
		}
		if endpointRegistry.GetMetrics(r.LBEndpoints[0].Host).DetectedTime().IsZero() {
			t.Fatal("failed to ignore negative duration")
		}
	})

	t.Run("endpoint already detected", func(t *testing.T) {
		const routes = `
			* -> fadeIn("1m") -> <"http://10.0.0.1:8080">
		`

		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		defer endpointRegistry.Close()
		rt, update := createRouting(t, routes, endpointRegistry)
		firstDetected := time.Now()

		const nextRoutes = `
			* -> fadeIn("1m") -> <"http://10.0.0.1:8080", "http://10.0.0.2:8080">
		`

		update(nextRoutes)
		r := route(rt, "/")

		var found bool
		for _, ep := range r.LBEndpoints {
			if ep.Host == "10.0.0.1:8080" {
				if endpointRegistry.GetMetrics(ep.Host).DetectedTime().After(firstDetected) {
					t.Fatal("Failed to keep detection time.")
				}

				found = true
			}
		}

		if !found {
			t.Fatal("Endpoint not found.")
		}
	})

	t.Run("endpoint temporarily disappears", func(t *testing.T) {
		const initialRoutes = `
			* -> fadeIn("1m") -> <"http://10.0.0.1:8080", "http://10.0.0.2:8080">
		`

		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		defer endpointRegistry.Close()
		rt, update := createRouting(t, initialRoutes, endpointRegistry)
		firstDetected := time.Now()

		const nextRoutes = `
			* -> fadeIn("1m") -> <"http://10.0.0.2:8080">
		`

		update(nextRoutes)
		update(initialRoutes)

		r := route(rt, "/")

		var found bool
		for _, ep := range r.LBEndpoints {
			if ep.Host == "10.0.0.1:8080" {
				if endpointRegistry.GetMetrics(ep.Host).DetectedTime().After(firstDetected) {
					t.Fatal("Failed to keep detection time.")
				}

				found = true
			}
		}

		if !found {
			t.Fatal("Endpoint not found.")
		}
	})

	t.Run("clear detected when gone for long enough", func(t *testing.T) {
		const initialRoutes = `
			* -> fadeIn("15ms") -> <"http://10.0.0.1:8080", "http://10.0.0.2:8080">
		`

		const lastSeenTimeout = 2 * time.Second
		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{LastSeenTimeout: lastSeenTimeout})
		defer endpointRegistry.Close()
		rt, update := createRouting(t, initialRoutes, endpointRegistry)
		firstDetected := time.Now()

		const nextRoutes = `
			* -> fadeIn("1m") -> <"http://10.0.0.2:8080">
		`

		time.Sleep(lastSeenTimeout + 10*time.Millisecond)
		update(nextRoutes)
		update(initialRoutes)

		r := route(rt, "/")

		var found bool
		for _, ep := range r.LBEndpoints {
			if ep.Host == "10.0.0.1:8080" {
				if !endpointRegistry.GetMetrics(ep.Host).DetectedTime().After(firstDetected) {
					t.Fatal("Failed to clear detection time.")
				}

				found = true
			}
		}

		if !found {
			t.Fatal("Endpoint not found.")
		}
	})

	t.Run("a more recent created time resets detection time", func(t *testing.T) {
		const routesFmt = `
			* -> fadeIn("1m") -> endpointCreated("http://10.0.0.1:8080", "%s") -> <"http://10.0.0.1:8080">
		`

		endpointRegistry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		defer endpointRegistry.Close()
		routes := fmt.Sprintf(routesFmt, nows(t))
		rt, update := createRouting(t, routes, endpointRegistry)
		firstDetected := time.Now()

		const nextRoutesFmt = `
			*
			-> fadeIn("1m")
			-> endpointCreated("http://10.0.0.1:8080", "%s")
			-> <"http://10.0.0.1:8080", "http://10.0.0.2:8080">
		`

		time.Sleep(15 * time.Millisecond)
		nextRoutes := fmt.Sprintf(nextRoutesFmt, nows(t))
		update(nextRoutes)
		r := route(rt, "/")

		var found bool
		for _, ep := range r.LBEndpoints {
			if ep.Host == "10.0.0.1:8080" {
				if !endpointRegistry.GetMetrics(ep.Host).DetectedTime().After(firstDetected) {
					t.Fatal("Failed to reset detection time.")
				}

				found = true
			}
		}

		if !found {
			t.Fatal("Endpoint not found.")
		}
	})
}
