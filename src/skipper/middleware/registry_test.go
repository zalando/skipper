package middleware

import (
	"skipper/mock"
	"testing"
)

func TestAddsGetsRemovesMiddleware(t *testing.T) {
	r := makeRegistry()

	mw1 := &mock.Middleware{"mw1"}
	mw2 := &mock.Middleware{"mw2"}
	mw3 := &mock.Middleware{"mw3"}
	r.Add(mw1, mw2, mw3)

	if r.Get("mw1") != mw1 ||
		r.Get("mw2") != mw2 ||
		r.Get("mw3") != mw3 {
		t.Error("failed to add/get middleware")
	}

	r.Remove("mw2")
	if r.Get("mw2") != nil {
		t.Error("failed to remove middleware")
	}
}
