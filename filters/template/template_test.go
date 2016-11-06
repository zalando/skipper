package template

import "testing"

type createTestItem struct {
	template string
	expected string
	getter   Getter
}

func testCreate(t *testing.T, items []createTestItem) {
	for _, ti := range items {
		func() {
			template := New(ti.template)
			result := template.Apply(ti.getter)
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
