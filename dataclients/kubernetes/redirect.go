package kubernetes

import (
	"fmt"
	"github.com/zalando/skipper/predicates"
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
)

const (
	redirectAnnotationKey     = "zalando.org/skipper-ingress-redirect"
	redirectCodeAnnotationKey = "zalando.org/skipper-ingress-redirect-code"
	forwardedProtoHeader      = "X-Forwarded-Proto"
)

type redirectInfo struct {
	defaultEnabled, enable, disable, override bool
	defaultCode, code                         int
	setHostCode                               map[string]int
	disableHost                               map[string]bool
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
	r.enable = !r.defaultEnabled && m.Annotations[redirectAnnotationKey] == "true"
	r.disable = r.defaultEnabled && m.Annotations[redirectAnnotationKey] == "false"

	r.code = r.defaultCode
	if annotationCode, ok := m.Annotations[redirectCodeAnnotationKey]; ok {
		var err interface{}
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

	r.override = r.defaultEnabled && !r.disable && r.code != r.defaultCode
}

func (r *redirectInfo) setHost(host string) {
	r.setHostCode[host] = r.code
}

func (r *redirectInfo) setHostDisabled(host string) {
	r.disableHost[host] = true
}

func (r *redirectInfo) updateHost(host string) {
	if r.enable || r.override {
		r.setHost(host)
	}

	if r.disable {
		r.setHostDisabled(host)
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
		Args: []interface{}{float64(1000)},
	}}, r.Predicates...)

	r.Filters = append(r.Filters, &eskip.Filter{
		Name: "redirectTo",
		Args: []interface{}{float64(code), "https:"},
	})

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
		Args: []interface{}{float64(1000)},
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
		Args: []interface{}{forwardedProtoHeader, "http"},
	})

	rr.Filters = append(rr.Filters, &eskip.Filter{
		Name: "redirectTo",
		Args: []interface{}{float64(code), "https:"},
	})

	return rr
}
