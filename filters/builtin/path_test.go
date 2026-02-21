package builtin

import (
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
	"net/http"
	"testing"
)

type createTestItem struct {
	msg  string
	args []any
	err  bool
}

func TestModifyPath(t *testing.T) {
	spec := NewModPath()
	f, err := spec.CreateFilter([]any{"/replace-this/", "/with-this/"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/path/replace-this/yo", nil)
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)
	if req.URL.Path != "/path/with-this/yo" {
		t.Error("failed to replace path")
	}
}

func TestModifyPathWithInvalidExpression(t *testing.T) {
	spec := NewModPath()
	if f, err := spec.CreateFilter([]any{"(?=;)", "foo"}); err == nil || f != nil {
		t.Error("Expected error for invalid regular expression parameter")
	}
}

func TestSetPath(t *testing.T) {
	spec := NewSetPath()
	f, err := spec.CreateFilter([]any{"/baz/qux"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/foo/bar", nil)
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)
	if req.URL.Path != "/baz/qux" {
		t.Error("failed to replace path")
	}
}

func TestSetPathWithTemplate(t *testing.T) {
	spec := NewSetPath()
	f, err := spec.CreateFilter([]any{"/path/${param2}/${param1}"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/foo/bar", nil)
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FRequest: req, FParams: map[string]string{
		"param1": "foo",
		"param2": "bar",
	}}

	f.Request(ctx)
	if req.URL.Path != "/path/bar/foo" {
		t.Error("failed to transform path")
	}
}

func testCreate(t *testing.T, spec func() filters.Spec, items []createTestItem) {
	for _, ti := range items {
		func() {
			f, err := spec().CreateFilter(ti.args)
			switch {
			case ti.err && err == nil:
				t.Error(ti.msg, "failed to fail")
			case !ti.err && err != nil:
				t.Error(ti.msg, err)
			case err == nil && f == nil:
				t.Error(ti.msg, "failed to create filter")
			}
		}()
	}
}

func TestCreateModPath(t *testing.T) {
	testCreate(t, NewModPath, []createTestItem{{
		"no args",
		nil,
		true,
	}, {
		"single arg",
		[]any{".*"},
		true,
	}, {
		"non-string arg, pos 1",
		[]any{3.14, "/foo"},
		true,
	}, {
		"non-string arg, pos 2",
		[]any{".*", 2.72},
		true,
	}, {
		"more than two args",
		[]any{".*", "/foo", "/bar"},
		true,
	}, {
		"create",
		[]any{".*", "/foo"},
		false,
	}})
}

func TestCreateSetPath(t *testing.T) {
	testCreate(t, NewSetPath, []createTestItem{{
		"no args",
		nil,
		true,
	}, {
		"non-string arg",
		[]any{3.14},
		true,
	}, {
		"more than one args",
		[]any{"/foo", "/bar"},
		true,
	}, {
		"create",
		[]any{"/foo"},
		false,
	}})
}
