package auth

import (
	"net/http"
	"time"

	"github.com/zalando/skipper/filters"
)

const (
	WebhookName = "webhook"
)

type WebhookOptions struct {
	Timeout      time.Duration
	MaxIdleConns int
}

type (
	webhookSpec struct {
		options WebhookOptions
	}
	webhookFilter struct {
		authClient *authClient
	}
)

// NewWebhook creates a new auth filter specification
// to validate authorization for requests via an
// external web hook.
func NewWebhook(timeout time.Duration) filters.Spec {
	return WebhookWithOptions(WebhookOptions{Timeout: timeout})
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
// string.
//
//     s.CreateFilter("https://my-auth-service.example.org/auth")
//
func (ws *webhookSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if l := len(args); l == 0 || l > 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	s, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	ac, err := newAuthClient(s, ws.options.Timeout, ws.options.MaxIdleConns)
	if err != nil {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &webhookFilter{authClient: ac}, nil
}

func copyHeader(to, from http.Header) {
	for k, v := range from {
		to[http.CanonicalHeaderKey(k)] = v
	}
}

func (f *webhookFilter) Request(ctx filters.FilterContext) {
	statusCode, err := f.authClient.getWebhook(ctx)
	if err != nil {
		unauthorized(ctx, WebhookName, authServiceAccess, f.authClient.url.Hostname())
	}
	// redirects, auth errors, webhook errors
	if statusCode >= 300 {
		unauthorized(ctx, WebhookName, invalidAccess, f.authClient.url.Hostname())
	}
	authorized(ctx, WebhookName)
}

func (*webhookFilter) Response(filters.FilterContext) {}

// Close cleans-up the quit channel used for this filter
func (f *webhookFilter) Close() {
	f.authClient.mu.Lock()
	if f.authClient.quit != nil {
		close(f.authClient.quit)
		f.authClient.quit = nil
	}
	f.authClient.mu.Unlock()
}
