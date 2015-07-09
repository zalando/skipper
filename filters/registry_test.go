package filters

import (
	"github.com/zalando/skipper/mock"
	"testing"
)

func TestAddsGetsRemovesFilterSpec(t *testing.T) {
	r := makeRegistry()

	mw1 := &mock.FilterSpec{"mw1"}
	mw2 := &mock.FilterSpec{"mw2"}
	mw3 := &mock.FilterSpec{"mw3"}
	r.Add(mw1, mw2, mw3)

	if r.Get("mw1") != mw1 ||
		r.Get("mw2") != mw2 ||
		r.Get("mw3") != mw3 {
		t.Error("failed to add/get filter spec")
	}

	r.Remove("mw2")
	if r.Get("mw2") != nil {
		t.Error("failed to remove filter spec")
	}
}
