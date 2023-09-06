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

func TestPostProcessorEndpointRegistry(t *testing.T) {
	createRouting := func(t *testing.T, routes string, registry *routing.EndpointRegistry) (*routing.Routing, func(string)) {
		dc, err := testdataclient.NewDoc(routes)
		if err != nil {
			t.Fatal(err)
		}

		if registry == nil {
			registry = routing.NewEndpointRegistry(routing.RegistryOptions{})
		}

		rt := routing.New(routing.Options{
			DataClients: []routing.DataClient{dc},
			FilterRegistry: filters.Registry{
				filters.FadeInName:          routing.NewFadeIn(),
				filters.EndpointCreatedName: routing.NewEndpointCreated(),
			},
			PostProcessors: []routing.PostProcessor{
				loadbalancer.NewAlgorithmProvider(),
				registry,
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

		registry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		rt, _ := createRouting(t, routes, registry)

		foo := route(rt, "/foo")
		if foo == nil || foo.LBFadeInDuration != 0 {
			t.Fatal("failed to preserve non-LB route")
		}

		bar := route(rt, "/bar")
		if bar == nil || bar.LBFadeInDuration != time.Minute {
			t.Fatal("failed to postprocess LB route")
		}

		for _, ep := range bar.LBEndpoints {
			if registry.GetMetrics(ep.Host).DetectedTime().IsZero() {
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

		rt, _ := createRouting(t, routes, nil)
		r := route(rt, "/")
		if r != nil {
			t.Fatal("created invalid LB endpoint")
		}
	})

	t.Run("negative duration", func(t *testing.T) {
		const routes = `
			* -> fadeIn("-1m") -> <"http://10.0.0.1:8080">
		`

		registry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		rt, _ := createRouting(t, routes, registry)
		r := route(rt, "/")
		if r == nil || len(r.LBEndpoints) == 0 || registry.GetMetrics(r.LBEndpoints[0].Host).DetectedTime().IsZero() {
			t.Fatal("failed to ignore negative duration")
		}
	})

	t.Run("endpoint already detected", func(t *testing.T) {
		const routes = `
			* -> fadeIn("1m") -> <"http://10.0.0.1:8080">
		`

		registry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		rt, update := createRouting(t, routes, registry)
		firstDetected := time.Now()

		const nextRoutes = `
			* -> fadeIn("1m") -> <"http://10.0.0.1:8080", "http://10.0.0.2:8080">
		`

		update(nextRoutes)
		r := route(rt, "/")

		var found bool
		for _, ep := range r.LBEndpoints {
			if ep.Host == "10.0.0.1:8080" {
				if registry.GetMetrics(ep.Host).DetectedTime().After(firstDetected) {
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

		registry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		rt, update := createRouting(t, initialRoutes, registry)
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
				if registry.GetMetrics(ep.Host).DetectedTime().After(firstDetected) {
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

		registry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		rt, update := createRouting(t, initialRoutes, registry)
		firstDetected := time.Now()

		const nextRoutes = `
			* -> fadeIn("1m") -> <"http://10.0.0.2:8080">
		`

		time.Sleep(time.Minute + time.Second)
		update(nextRoutes)
		update(initialRoutes)

		r := route(rt, "/")

		var found bool
		for _, ep := range r.LBEndpoints {
			if ep.Host == "10.0.0.1:8080" {
				if !registry.GetMetrics(ep.Host).DetectedTime().After(firstDetected) {
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

		registry := routing.NewEndpointRegistry(routing.RegistryOptions{})
		routes := fmt.Sprintf(routesFmt, nows(t))
		rt, update := createRouting(t, routes, registry)
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
				if !registry.GetMetrics(ep.Host).DetectedTime().After(firstDetected) {
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
