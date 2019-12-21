package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/http/httpguts"

	"github.com/zalando/skipper/filters"
)

const (
	WebhookName = "webhook"
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
	return WebhookName
}

// CreateFilter creates an auth filter. The first argument is an URL
// string. The second, optional, argument is a comma separated list of
// headers to forward from from webhook response.
//
//     s.CreateFilter("https://my-auth-service.example.org/auth", "X-Auth-User,X-Auth-User-Roles")
//
func (ws *webhookSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if l := len(args); l == 0 || l > 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

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

	ac, err := newAuthClient(s, webhookSpanName, ws.options.Timeout, ws.options.MaxIdleConns, ws.options.Tracer)
	if err != nil {
		return nil, filters.ErrInvalidFilterParameters
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
		log.Errorf("Failed to make authentication webhook request: %v.", err)
	}

	// errors, redirects, auth errors, webhook errors
	if err != nil || resp.StatusCode >= 300 {
		unauthorized(ctx, "", invalidAccess, f.authClient.url.Hostname(), WebhookName)
		return
	}

	// copy required headers from webhook response into the current request
	for _, hk := range f.forwardResponseHeaderKeys {
		if h, ok := resp.Header[hk]; ok {
			ctx.Request().Header[hk] = h
		}
	}

	authorized(ctx, WebhookName)
}

func (*webhookFilter) Response(filters.FilterContext) {}

// Close cleans-up the authClient
func (f *webhookFilter) Close() {
	f.authClient.Close()
}
