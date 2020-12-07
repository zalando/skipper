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

var parameterRegexp = regexp.MustCompile(`\$\{(\w+)\}`)

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
	matches := parameterRegexp.FindAllStringSubmatch(template, -1)
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
	return t.apply(requestGetter(ctx))
}

// ApplyResponseContext evaluates the template using a filter context, request and response attributes to resolve the
// placeholders. Returns true if all placeholders resolved to non-empty values.
func (t *Template) ApplyResponseContext(ctx filters.FilterContext) (string, bool) {
	return t.apply(responseGetter(ctx))
}

func requestGetter(ctx filters.FilterContext) func(key string) string {
	return func(key string) string {
		if v := ctx.PathParam(key); v != "" {
			return v
		}
		return ""
	}
}

func responseGetter(ctx filters.FilterContext) func(key string) string {
	// same as request for now
	return requestGetter(ctx)
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
