package traffic

import (
	"net/http"
	"testing"
)

const (
	defaultTrafficGroupCookie = "testcookie"
)

func TestCreate(t *testing.T) {
	for _, ti := range []struct {
		msg   string
		args  []interface{}
		check predicate
		err   bool
	}{{
		"no args",
		nil,
		predicate{},
		true,
	}, {
		"too many args",
		[]interface{}{.3, "testname", "group", "something"},
		predicate{},
		true,
	}, {
		"first not number",
		[]interface{}{"something"},
		predicate{},
		true,
	}, {
		"second not string",
		[]interface{}{.3, .2},
		predicate{},
		true,
	}, {
		"third not string",
		[]interface{}{.3, .2, "group"},
		predicate{},
		true,
	}, {
		"you have 1 parameter but need to have 3 parameters",
		[]interface{}{.3},
		predicate{chance: .3},
		true,
	}, {
		"you have 2 parameters but need to have 3 parameters",
		[]interface{}{.3, "foo"},
		predicate{chance: .3},
		true,
	}, {
		"chance and group",
		[]interface{}{.3, "testname", "group"},
		predicate{chance: .3, trafficGroup: "group", trafficGroupCookie: "testname"},
		false,
	}, {
		"chance and group and cookie name",
		[]interface{}{.3, "test"},
		predicate{chance: .3, trafficGroupCookie: "test"},
		true,
	}} {
		pi, err := (&spec{}).Create(ti.args)
		if err == nil && ti.err || err != nil && !ti.err {
			t.Error(ti.msg, "failure case", err, ti.err)
		} else if err == nil {
			p := pi.(*predicate)
			if p.chance != ti.check.chance {
				t.Error(ti.msg, "chance", p.chance, ti.check.chance)
			}

			if p.trafficGroup != ti.check.trafficGroup {
				t.Error(ti.msg, "traffic group", p.trafficGroup, ti.check.trafficGroup)
			}

			if p.trafficGroupCookie != ti.check.trafficGroupCookie {
				t.Error(ti.msg, "traffic group cookie", p.trafficGroupCookie, ti.check.trafficGroupCookie)
			}
		}
	}
}

func TestMatch(t *testing.T) {
	for _, ti := range []struct {
		msg   string
		p     predicate
		r     http.Request
		match bool
	}{{
		"not sticky, no match",
		predicate{chance: 0},
		http.Request{},
		false,
	}, {
		"not sticky, match",
		predicate{chance: 1},
		http.Request{},
		true,
	}, {
		"sticky, has cookie, no match",
		predicate{chance: 1, trafficGroup: "A", trafficGroupCookie: defaultTrafficGroupCookie},
		http.Request{Header: http.Header{"Cookie": []string{defaultTrafficGroupCookie + "=B"}}},
		false,
	}, {
		"sticky, has cookie, match",
		predicate{chance: 0, trafficGroup: "A", trafficGroupCookie: defaultTrafficGroupCookie},
		http.Request{Header: http.Header{"Cookie": []string{defaultTrafficGroupCookie + "=A"}}},
		true,
	}, {
		"sticky, no cookie, no match",
		predicate{chance: 0, trafficGroup: "A", trafficGroupCookie: defaultTrafficGroupCookie},
		http.Request{Header: http.Header{}},
		false,
	}, {
		"sticky, no cookie, match",
		predicate{chance: 1, trafficGroup: "A", trafficGroupCookie: defaultTrafficGroupCookie},
		http.Request{Header: http.Header{}},
		true,
	}} {
		if (&ti.p).Match(&ti.r) != ti.match {
			t.Error(ti.msg)
		}
	}
}
