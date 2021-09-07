package main

import (
	"net/http"
	"time"

	"github.com/zalando/skipper/cmd/routesrv/options"
	"github.com/zalando/skipper/config"
	"github.com/zalando/skipper/dataclients/kubernetes"
)

func run(o options.Options) error {
	dataclient, err := kubernetes.New(kubernetes.Options{
		KubernetesInCluster:               o.KubernetesInCluster,
		KubernetesURL:                     o.KubernetesURL,
		ProvideHealthcheck:                o.KubernetesHealthcheck,
		ProvideHTTPSRedirect:              o.KubernetesHTTPSRedirect,
		HTTPSRedirectCode:                 o.KubernetesHTTPSRedirectCode,
		IngressClass:                      o.KubernetesIngressClass,
		RouteGroupClass:                   o.KubernetesRouteGroupClass,
		ReverseSourcePredicate:            o.ReverseSourcePredicate,
		WhitelistedHealthCheckCIDR:        o.WhitelistedHealthCheckCIDR,
		PathMode:                          o.KubernetesPathMode,
		KubernetesNamespace:               o.KubernetesNamespace,
		KubernetesEnableEastWest:          o.KubernetesEnableEastWest,
		KubernetesEastWestDomain:          o.KubernetesEastWestDomain,
		KubernetesEastWestRangeDomains:    o.KubernetesEastWestRangeDomains,
		KubernetesEastWestRangePredicates: o.KubernetesEastWestRangePredicates,
		DefaultFiltersDir:                 o.DefaultFiltersDir,
		OriginMarker:                      o.EnableRouteCreationMetrics,
		BackendNameTracingTag:             o.OpenTracingBackendNameTag,
		OnlyAllowedExternalNames:          o.KubernetesOnlyAllowedExternalNames,
		AllowedExternalNames:              o.KubernetesAllowedExternalNames,
	})
	if err != nil {
		return err
	}

	cache := newCache(dataclient, 3*time.Second)

	http.HandleFunc("/routes", func(w http.ResponseWriter, r *http.Request) {
		w.Write(cache.get())
	})

	http.ListenAndServe(":8080", nil)

	return nil
}

func main() {
	cfg := config.NewConfig()
	cfg.Parse()
	run(cfg.ToRouteSrvOptions())
}
