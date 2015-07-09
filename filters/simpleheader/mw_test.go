package simpleheader

import "testing"

func TestCreatesFilterSpec(t *testing.T) {
	mw := &Type{}
	if mw.Name() != "_simpleheader" {
		t.Error("wrong name")
	}
}

func TestCreatesFilter(t *testing.T) {
	mw := &Type{}
	f, err := mw.MakeFilter("filter", []interface{}{"X-Test", "test-value"})
	if err != nil || f.Id() != "filter" {
		t.Error("failed to create filter")
	}
}

func TestReportsMissingArg(t *testing.T) {
	f := &Type{}
	err := f.InitFilter("filter", []interface{}{"test-value"})
	if err == nil {
		t.Error("failed to fail on missing key")
	}
}

func TestReturnsKeyAndValue(t *testing.T) {
	f := &Type{}
	f.InitFilter("filter", []interface{}{"X-Test", "test-value"})
	if f.Key() != "X-Test" || f.Value() != "test-value" {
		t.Error("failed to return key and value")
	}
}
