package accesslog

import (
	"testing"

	"github.com/zalando/skipper/filters/filtertest"
)

func Test(t *testing.T) {
	const state = "false"

	f, err := NewAccessLog().CreateFilter([]interface{}{state})
	if err != nil {
		t.Fatal(err)
	}

	var ctx filtertest.Context
	ctx.FStateBag = make(map[string]interface{})

	f.Request(&ctx)
	bag := ctx.StateBag()
	if bag[AccessLogEnabledKey] != false {
		t.Error("failed to set access log state")
	}
}
