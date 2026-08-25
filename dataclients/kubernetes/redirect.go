package kubernetes

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/predicates"
)

const (
	redirectAnnotationKey     = "zalando.org/skipper-ingress-redirect"
	redirectCodeAnnotationKey = "zalando.org/skipper-ingress-redirect-code"
	forwardedProtoHeader      = "X-Forwarded-Proto"
)

type redirectInfo struct {
	defaultEnabled, enable, disable, ignore bool
	defaultCode, code                       int
	setHostCode                             map[string]int
	disableHost                             map[string]bool
}

func createRedirectInfo(defaultEnabled bool, defaultCode int) *redirectInfo {
	return &redirectInfo{
		defaultEnabled: defaultEnabled,
		defaultCode:    defaultCode,
		setHostCode:    make(map[string]int),
		disableHost:    make(map[string]bool),
	}
}

func (r *redirectInfo) initCurrent(m *definitions.Metadata) {
	r.enable = m.Annotations[redirectAnnotationKey] == "true"
	r.disable = m.Annotations[redirectAnnotationKey] == "false"
	r.ignore = strings.Contains(m.Annotations[definitions.IngressPredicateAnnotation], `Header("X-Forwarded-Proto"`) || strings.Contains(m.Annotations[definitions.IngressRoutesAnnotation], `Header("X-Forwarded-Proto"`)

	r.code = r.defaultCode
	if annotationCode, ok := m.Annotations[redirectCodeAnnotationKey]; ok {
		var err any
		if r.code, err = strconv.Atoi(annotationCode); err != nil ||
			r.code < http.StatusMultipleChoices ||
			r.code >= http.StatusBadRequest {

			if err == nil {
				err = annotationCode
			}

			log.Error("invalid redirect code annotation:", err)
			r.code = r.defaultCode
		}
	}
}

func (r *redirectInfo) setHost(host string) {
	r.setHostCode[host] = r.code
}

func (r *redirectInfo) setHostDisabled(host string) {
	r.disableHost[host] = true
}

func (r *redirectInfo) updateHost(host string) {
	switch {
	case r.enable:
		r.setHost(host)
	case r.disable:
		r.setHostDisabled(host)
	case r.defaultEnabled:
		r.setHost(host)
	}
}

func routeIDForRedirectRoute(baseID string, enable bool) string {
	f := "%s_https_redirect"
	if !enable {
		f = "%s_disable_https_redirect"
	}

	return fmt.Sprintf(f, baseID)
}

func initRedirectRoute(r *eskip.Route, code int) {
	if r.Headers == nil {
		r.Headers = make(map[string]string)
	}
	r.Headers[forwardedProtoHeader] = "http"

	// Give this route a higher weight so that it will get precedence over existing routes
	r.Predicates = append([]*eskip.Predicate{{
		Name: predicates.WeightName,
		Args: []any{float64(1000)},
	}}, r.Predicates...)

	// remove all filters and just set redirect filter
	r.Filters = []*eskip.Filter{
		{
			Name: "redirectTo",
			Args: []any{float64(code), "https:"},
		},
	}

	r.BackendType = eskip.ShuntBackend
	r.Backend = ""
}

func initDisableRedirectRoute(r *eskip.Route) {
	if r.Headers == nil {
		r.Headers = make(map[string]string)
	}
	r.Headers[forwardedProtoHeader] = "http"

	// Give this route a higher weight so that it will get precedence over existing routes
	r.Predicates = append([]*eskip.Predicate{{
		Name: predicates.WeightName,
		Args: []any{float64(1000)},
	}}, r.Predicates...)
}

func globalRedirectRoute(code int) *eskip.Route {
	r := &eskip.Route{Id: httpRedirectRouteID}
	initRedirectRoute(r, code)
	return r
}

func createIngressEnableHTTPSRedirect(r *eskip.Route, code int) *eskip.Route {
	rr := *r
	rr.Id = routeIDForRedirectRoute(rr.Id, true)
	initRedirectRoute(&rr, code)
	return &rr
}

func createIngressDisableHTTPSRedirect(r *eskip.Route) *eskip.Route {
	rr := *r
	rr.Id = routeIDForRedirectRoute(rr.Id, false)
	initDisableRedirectRoute(&rr)
	return &rr
}

func hasProtoPredicate(r *eskip.Route) bool {
	if r.Headers != nil {
		for name := range r.Headers {
			if http.CanonicalHeaderKey(name) == forwardedProtoHeader {
				return true
			}
		}
	}

	if r.HeaderRegexps != nil {
		for name := range r.HeaderRegexps {
			if http.CanonicalHeaderKey(name) == forwardedProtoHeader {
				return true
			}
		}
	}

	for _, p := range r.Predicates {
		if p.Name != "Header" && p.Name != "HeaderRegexp" {
			continue
		}

		if len(p.Args) > 0 && p.Args[0] == forwardedProtoHeader {
			return true
		}
	}

	return false
}

func createHTTPSRedirect(code int, r *eskip.Route) *eskip.Route {
	// copy to avoid unexpected mutations
	rr := eskip.Copy(r)
	rr.Id = routeIDForRedirectRoute(rr.Id, true)
	rr.BackendType = eskip.ShuntBackend

	rr.Predicates = append(rr.Predicates, &eskip.Predicate{
		Name: "Header",
		Args: []any{forwardedProtoHeader, "http"},
	})

	// remove all filters and just set redirect filter
	rr.Filters = []*eskip.Filter{
		{
			Name: "redirectTo",
			Args: []any{float64(code), "https:"},
		},
	}

	return rr
}
