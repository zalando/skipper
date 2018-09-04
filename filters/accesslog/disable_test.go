package accesslog

import (
	"testing"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/filtertest"
)

func TestEnabled(t *testing.T) {
	const state = "false"

	f, err := NewAccessLogDisabled().CreateFilter([]interface{}{state})
	if err != nil {
		t.Fatal(err)
	}

	var ctx filtertest.Context
	ctx.FStateBag = make(map[string]interface{})

	f.Request(&ctx)
	bag := ctx.StateBag()
	if bag[AccessLogDisabledKey] != false {
		t.Error("failed to set access log state")
	}
}

func TestDisabled(t *testing.T) {
	const state = "true"

	f, err := NewAccessLogDisabled().CreateFilter([]interface{}{state})
	if err != nil {
		t.Fatal(err)
	}

	var ctx filtertest.Context
	ctx.FStateBag = make(map[string]interface{})

	f.Request(&ctx)
	bag := ctx.StateBag()
	if bag[AccessLogDisabledKey] != true {
		t.Error("failed to set access log state")
	}
}

func TestUnknownValue(t *testing.T) {
	const state = "unknownValue"

	_, err := NewAccessLogDisabled().CreateFilter([]interface{}{state})

	if err == nil || err != filters.ErrInvalidFilterParameters {
		t.Error("should throw error on unknown value")
	}
}

func TestMissingValue(t *testing.T) {
	_, err := NewAccessLogDisabled().CreateFilter([]interface{}{})

	if err == nil || err != filters.ErrInvalidFilterParameters {
		t.Error("should throw error on missing value")
	}
}
