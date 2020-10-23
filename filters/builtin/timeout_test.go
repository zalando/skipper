package builtin

import (
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestBackendTimeout(t *testing.T) {
	bt := NewBackendTimeout()
	if bt.Name() != BackendTimeoutName {
		t.Error("wrong name")
	}

	f, err := bt.CreateFilter([]interface{}{"2s"})
	if err != nil {
		t.Error("wrong id")
	}

	c := &filtertest.Context{FRequest: &http.Request{}, FStateBag: make(map[string]interface{})}
	f.Request(c)

	if c.FStateBag[filters.BackendTimeout] != 2*time.Second {
		t.Error("wrong timeout")
	}

	// second filter overwrites
	f, _ = bt.CreateFilter([]interface{}{"5s"})
	f.Request(c)

	if c.FStateBag[filters.BackendTimeout] != 5*time.Second {
		t.Error("overwrite expected")
	}
}
