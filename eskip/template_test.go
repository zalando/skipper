package eskip

import (
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
)

type createTestItem struct {
	template string
	expected string
	getter   TemplateGetter
}

func testCreate(t *testing.T, items []createTestItem) {
	for _, ti := range items {
		func() {
			template := NewTemplate(ti.template)
			result := template.Apply(ti.getter)
			if result != ti.expected {
				t.Errorf("Error: '%s' != '%s'", result, ti.expected)
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

func TestTemplateApplyRequestResponseContext(t *testing.T) {
	for _, ti := range []struct {
		name           string
		template       string
		context        *filtertest.Context
		requestExpect  string
		requestOk      bool
		responseExpect string
		responseOk     bool
	}{{
		"path params",
		"hello ${p1} ${p2}",
		&filtertest.Context{
			FParams: map[string]string{
				"p1": "path",
				"p2": "params",
			},
		},
		"hello path params",
		true,
		"hello path params",
		true,
	}, {
		"all missing",
		"hello ${p1} ${p2}",
		&filtertest.Context{},
		"hello  ",
		false,
		"hello  ",
		false,
	}, {
		"some missing",
		"hello ${p1} ${missing}",
		&filtertest.Context{
			FParams: map[string]string{
				"p1": "X",
			},
		},
		"hello X ",
		false,
		"hello X ",
		false,
	},
	} {
		t.Run(ti.name, func(t *testing.T) {
			template := NewTemplate(ti.template)
			result, ok := template.ApplyRequestContext(ti.context)
			if result != ti.requestExpect || ok != ti.requestOk {
				t.Errorf("Apply request context result mismatch: '%s' != '%s'", result, ti.requestExpect)
			}

			result, ok = template.ApplyResponseContext(ti.context)
			if result != ti.responseExpect || ok != ti.responseOk {
				t.Errorf("Apply response context result mismatch: '%s' != '%s'", result, ti.responseExpect)
			}
		})
	}
}
