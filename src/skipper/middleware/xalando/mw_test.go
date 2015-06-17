package xalando

import "testing"

func TestName(t *testing.T) {
	mw := &impl{}
	if mw.Name() != "xalando" {
		t.Error("wrong name")
	}
}
