package helpers

import (
	"regexp"
	"strings"
)

const ()

var (
	parameterRegexp = regexp.MustCompile("\\$\\{(\\w+)\\}")
)

type TemplateGetter func(string) string

type TemplateString struct {
	template     string
	placeholders []string
}

func NewTemplateString(template string) *TemplateString {
	matches := parameterRegexp.FindAllStringSubmatch(template, -1)
	placeholders := make([]string, len(matches))

	for index, placeholder := range matches {
		placeholders[index] = placeholder[1]
	}

	return &TemplateString{template: template, placeholders: placeholders}
}

func (t *TemplateString) ApplyWithGetter(getParamValue TemplateGetter) string {
	result := t.template

	if getParamValue == nil {
		return result
	}

	for _, placeholder := range t.placeholders {
		result = strings.Replace(result, "${"+placeholder+"}", getParamValue(placeholder), -1)
	}

	return result
}
