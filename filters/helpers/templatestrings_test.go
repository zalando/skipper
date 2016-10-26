package helpers

import (
	"testing"
)

type createTestItem struct {
	template string
	expected string
	getter   TemplateGetter
}

func testCreate(t *testing.T, items []createTestItem) {
	for _, ti := range items {
		func() {
			template := NewTemplateString(ti.template)
			result := template.ApplyWithGetter(ti.getter)
			if result != ti.expected {
				t.Error(`Error: "` + result + `" != "` + ti.expected + `"`)
			}
		}()
	}
}

func TestTemplateGetter(t *testing.T) {
	testCreate(t, []createTestItem{{
		"template",
		"template",
		func(param string) string {
			return param
		},
	}, {
		"/path/${param1}/",
		"/path/param1/",
		func(param string) string {
			return param
		},
	}, {
		"/${param2}/${param1}/",
		"/param2/param1/",
		func(param string) string {
			return param
		},
	}, {
		"/${param2}",
		"/param2",
		func(param string) string {
			return param
		},
	}, {
		"/${missing}",
		"/",
		func(param string) string {
			return ""
		},
	}, {
		"/${param1}",
		"/${param1}",
		nil,
	}})
}
