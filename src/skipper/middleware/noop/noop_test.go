package noop

import (
	"net/http"
	"testing"
)

func TestCreatesNoopMiddlewareAndFilter(t *testing.T) {
	n := &Type{}
	if n.Name() != "noop" {
		t.Error("wrong name")
	}

	f, err := n.MakeFilter("id", nil)
	if err != nil || f.Id() != "id" {
		t.Error("wrong id")
	}

	req := &http.Request{}
	if f.ProcessRequest(req) != req {
		t.Error("failed not to process request")
	}

	rsp := &http.Response{}
	if f.ProcessResponse(rsp) != rsp {
		t.Error("failed not to process response")
	}
}
