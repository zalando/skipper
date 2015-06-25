package noop

import (
	"net/http"
	"skipper/mock"
	"testing"
)

func TestCreatesNoopFilterSpecAndFilter(t *testing.T) {
	n := &Type{}
	if n.Name() != "_noop" {
		t.Error("wrong name")
	}

	f, err := n.MakeFilter("id", nil)
	if err != nil || f.Id() != "id" {
		t.Error("wrong id")
	}

	req := &http.Request{}
	c := &mock.FilterContext{nil, req, nil}
	f.Request(c)

	rsp := &http.Response{}
	c.FResponse = rsp
	f.Response(c)
}
