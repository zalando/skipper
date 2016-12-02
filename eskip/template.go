// Package template provides a simple templating solution reusable in filters.
//
// (Note that the current template syntax is EXPERIMENTAL, and may change in
// the near future.)
package eskip

import (
	"regexp"
	"strings"
)

var parameterRegexp = regexp.MustCompile("\\$\\{(\\w+)\\}")

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
	result := t.template

	if get == nil {
		return result
	}

	for _, placeholder := range t.placeholders {
		result = strings.Replace(result, "${"+placeholder+"}", get(placeholder), -1)
	}

	return result
}
