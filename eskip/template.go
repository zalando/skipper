// Package template provides a simple templating solution reusable in filters.
//
// (Note that the current template syntax is EXPERIMENTAL, and may change in
// the near future.)
package eskip

import (
	"net/http"
	"regexp"
	"strings"
)

var placeholderRegexp = regexp.MustCompile(`\$\{([^{}]+)\}`)

// TemplateGetter functions return the value for a template parameter name.
type TemplateGetter func(string) string

// Template represents a string template with named placeholders.
type Template struct {
	template     string
	placeholders []string
}

type TemplateContext interface {
	PathParam(string) string

	Request() *http.Request

	Response() *http.Response
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

// ApplyContext evaluates the template using template context to resolve the
// placeholders. Returns true if all placeholders resolved to non-empty values.
func (t *Template) ApplyContext(ctx TemplateContext) (string, bool) {
	return t.apply(func(key string) string {
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
		if ctx.Response() != nil {
			if h := strings.TrimPrefix(key, "response.header."); h != key {
				return ctx.Response().Header.Get(h)
			}
		}
		return ctx.PathParam(key)
	})
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
