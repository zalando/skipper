// Package template provides a simple templating solution reusable in filters.
//
// (Note that the current template syntax is EXPERIMENTAL, and may change in
// the near future.)
package eskip

import (
	"regexp"
	"strings"

	"github.com/zalando/skipper/filters"
)

var placeholderRegexp = regexp.MustCompile(`\$\{([^{}]+)\}`)

// TemplateGetter functions return the value for a template parameter name.
type TemplateGetter func(string) string

// Template represents a string template with named placeholders.
type Template struct {
	template     string
	placeholders []string
}

// New parses a template string and returns a reusable *Template object.
// The template string can contain named placeholders of the format:
//
// 	Hello, ${who}!
//
func NewTemplate(template string) *Template {
	matches := placeholderRegexp.FindAllStringSubmatch(template, -1)
	placeholders := make([]string, len(matches))

	for index, placeholder := range matches {
		placeholders[index] = placeholder[1]
	}

	return &Template{template: template, placeholders: placeholders}
}

// Apply evaluates the template using a TemplateGetter function to resolve the
// placeholders.
func (t *Template) Apply(get TemplateGetter) string {
	if get == nil {
		return t.template
	}
	result, _ := t.apply(get)
	return result
}

// ApplyRequestContext evaluates the template using a filter context and request attributes to resolve the
// placeholders. Returns true if all placeholders resolved to non-empty values.
func (t *Template) ApplyRequestContext(ctx filters.FilterContext) (string, bool) {
	return t.apply(contextGetter(ctx, false))
}

// ApplyResponseContext evaluates the template using a filter context, request and response attributes to resolve the
// placeholders. Returns true if all placeholders resolved to non-empty values.
func (t *Template) ApplyResponseContext(ctx filters.FilterContext) (string, bool) {
	return t.apply(contextGetter(ctx, true))
}

func contextGetter(ctx filters.FilterContext, response bool) func(key string) string {
	return func(key string) string {
		if h := strings.TrimPrefix(key, "request.header."); h != key {
			return ctx.Request().Header.Get(h)
		}
		if q := strings.TrimPrefix(key, "request.query."); q != key {
			return ctx.Request().URL.Query().Get(q)
		}
		if c := strings.TrimPrefix(key, "request.cookie."); c != key {
			if cookie, err := ctx.Request().Cookie(c); err == nil {
				return cookie.Value
			}
			return ""
		}
		if key == "request.path" {
			return ctx.Request().URL.Path
		}
		if response {
			if h := strings.TrimPrefix(key, "response.header."); h != key {
				return ctx.Response().Header.Get(h)
			}
		}
		return ctx.PathParam(key)
	}
}

// apply evaluates the template using a TemplateGetter function to resolve the
// placeholders. Returns true if all placeholders resolved to non-empty values.
func (t *Template) apply(get TemplateGetter) (string, bool) {
	result := t.template
	missing := false
	for _, placeholder := range t.placeholders {
		value := get(placeholder)
		if value == "" {
			missing = true
		}
		result = strings.Replace(result, "${"+placeholder+"}", value, -1)
	}
	return result, !missing
}
