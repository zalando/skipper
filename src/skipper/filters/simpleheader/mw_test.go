package simpleheader

import "testing"

func TestCreatesFilterSpec(t *testing.T) {
	mw := &Type{}
	if mw.Name() != "_simple-header" {
		t.Error("wrong name")
	}
}

func TestCreatesFilter(t *testing.T) {
	mw := &Type{}
	f, err := mw.MakeFilter("filter", map[string]interface{}{"key": "X-Test", "value": "test-value"})
	if err != nil || f.Id() != "filter" {
		t.Error("failed to create filter")
	}
}

func TestReportsMissingKey(t *testing.T) {
	f := &Type{}
	err := f.InitFilter("filter", map[string]interface{}{"value": "test-value"})
	if err == nil {
		t.Error("failed to fail on missing key")
	}
}

func TestReportsMissingValue(t *testing.T) {
	f := &Type{}
	err := f.InitFilter("filter", map[string]interface{}{"key": "X-Test"})
	if err == nil {
		t.Error("failed to fail on missing value")
	}
}

func TestReturnsKeyAndValue(t *testing.T) {
	f := &Type{}
	f.InitFilter("filter", map[string]interface{}{"key": "X-Test", "value": "test-value"})
	if f.Key() != "X-Test" || f.Value() != "test-value" {
		t.Error("failed to return key and value")
	}
}
