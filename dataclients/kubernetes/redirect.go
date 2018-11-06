package kubernetes

import (
	"fmt"
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
)

const (
	redirectAnnotationKey     = "zalando.org/skipper-ingress-redirect"
	redirectCodeAnnotationKey = "zalando.org/skipper-ingress-redirect-code"
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

func (r *redirectInfo) initCurrent(m *metadata) {
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
	r.Headers["X-Forwarded-Proto"] = "http"

	// the below duplicate any-path (.*) is set to make sure that
	// the redirect route has a higher priority during matching than
	// the normal routes that may have max 2 predicates: path regexp
	// and host.
	//
	// A better solution might be to implement a Weight() predicate
	// and apply it here.
	//
	r.PathRegexps = append(r.PathRegexps, ".*")
	r.PathRegexps = append(r.PathRegexps, ".*")

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
	r.Headers["X-Forwarded-Proto"] = "http"

	// the below duplicate any-path (.*) is set to make sure that
	// the redirect route has a higher priority during matching than
	// the normal routes that may have max 2 predicates: path regexp
	// and host.
	//
	// A better solution might be to implement a Weight() predicate
	// and apply it here.
	//
	r.PathRegexps = append(r.PathRegexps, ".*")
	r.PathRegexps = append(r.PathRegexps, ".*")
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
