package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/opentracing/opentracing-go"
	"golang.org/x/net/http/httpguts"

	"github.com/zalando/skipper/filters"
)

const (
	// Deprecated, use filters.WebhookName instead
	WebhookName = filters.WebhookName
)

type WebhookOptions struct {
	Timeout      time.Duration
	MaxIdleConns int
	Tracer       opentracing.Tracer
}

type (
	webhookSpec struct {
		options WebhookOptions
	}
	webhookFilter struct {
		authClient                *authClient
		forwardResponseHeaderKeys []string
	}
)

var webhookAuthClient map[string]*authClient = make(map[string]*authClient)

// NewWebhook creates a new auth filter specification
// to validate authorization for requests via an
// external web hook.
func NewWebhook(timeout time.Duration) filters.Spec {
	return WebhookWithOptions(WebhookOptions{Timeout: timeout, Tracer: opentracing.NoopTracer{}})
}

// WebhookWithOptions creates a new auth filter specification
// to validate authorization of requests via an external web
// hook.
func WebhookWithOptions(o WebhookOptions) filters.Spec {
	return &webhookSpec{options: o}
}

func (*webhookSpec) Name() string {
	return filters.WebhookName
}

// CreateFilter creates an auth filter. The first argument is an URL
// string. The second, optional, argument is a comma separated list of
// headers to forward from webhook response.
//
//	s.CreateFilter("https://my-auth-service.example.org/auth")
//	s.CreateFilter("https://my-auth-service.example.org/auth", "X-Auth-User,X-Auth-User-Roles")
func (ws *webhookSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if l := len(args); l == 0 || l > 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	var ok bool
	s, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	forwardResponseHeaderKeys := make([]string, 0)

	if len(args) > 1 {
		// Capture headers that should be forwarded from webhook responses.
		headerKeysOption, ok := args[1].(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}

		headerKeys := strings.Split(headerKeysOption, ",")

		for _, header := range headerKeys {
			valid := httpguts.ValidHeaderFieldName(header)
			if !valid {
				return nil, fmt.Errorf("header %s is invalid", header)
			}
			forwardResponseHeaderKeys = append(forwardResponseHeaderKeys, http.CanonicalHeaderKey(header))
		}
	}

	var ac *authClient
	var err error
	if ac, ok = webhookAuthClient[s]; !ok {
		ac, err = newAuthClient(s, webhookSpanName, ws.options.Timeout, ws.options.MaxIdleConns, ws.options.Tracer, true)
		if err != nil {
			return nil, filters.ErrInvalidFilterParameters
		}
		webhookAuthClient[s] = ac
	}

	return &webhookFilter{authClient: ac, forwardResponseHeaderKeys: forwardResponseHeaderKeys}, nil
}

func copyHeader(to, from http.Header) {
	for k, v := range from {
		to[http.CanonicalHeaderKey(k)] = v
	}
}

func (f *webhookFilter) Request(ctx filters.FilterContext) {
	resp, err := f.authClient.getWebhook(ctx)
	if err != nil {
		ctx.Logger().Errorf("Failed to make authentication webhook request: %v.", err)
	}

	// forbidden
	if err == nil && resp.StatusCode == http.StatusForbidden {
		forbidden(ctx, "", invalidScope, filters.WebhookName)
		return
	}

	// redirect
	if err == nil && resp.StatusCode == http.StatusFound {
		redirect(ctx, "", invalidAccess, resp.Header.Get("Location"), filters.WebhookName)
		return
	}

	// errors, auth errors, webhook errors
	if err != nil || resp.StatusCode >= 300 {
		unauthorized(ctx, "", invalidAccess, f.authClient.url.Hostname(), filters.WebhookName)
		return
	}

	// copy required headers from webhook response into the current request
	for _, hk := range f.forwardResponseHeaderKeys {
		if h, ok := resp.Header[hk]; ok {
			ctx.Request().Header[hk] = h
		}
	}

	authorized(ctx, filters.WebhookName)
}

func (*webhookFilter) Response(filters.FilterContext) {}
