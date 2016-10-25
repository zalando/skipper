package auth

import (
	"github.com/zalando/skipper/filters/filtertest"
	"net/http"
	"testing"
)

type createTestItem struct {
	msg  string
	args []interface{}
	err  bool
}

func TestWithFailingAuth(t *testing.T) {
	spec := NewBasicAuth()
	f, err := spec.CreateFilter([]interface{}{"htpasswd.test"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/", nil)
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)
	if ctx.Response().Header.Get(ForceBasicAuthHeaderName) != ForceBasicAuthHeaderValue && ctx.Served() {
		t.Error("Authentication header wrong/missing")
	}
}

func TestWithSuccessfulAuth(t *testing.T) {
	spec := NewBasicAuth()
	f, err := spec.CreateFilter([]interface{}{"htpasswd.test"})
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest("GET", "https://www.example.org/", nil)
	req.SetBasicAuth("myName", "myPassword")
	if err != nil {
		t.Error(err)
	}

	ctx := &filtertest.Context{FRequest: req}
	f.Request(ctx)
	if ctx.Served() {
		t.Error("Authentication not successful")
	}
}
