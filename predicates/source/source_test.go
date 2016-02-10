package source

import (
	"net/http"
	"testing"
)

func TestCreate(t *testing.T) {
	for _, ti := range []struct {
		msg  string
		args []interface{}
		err  bool
	}{{
		"no args",
		nil,
		true,
	}, {
		"arg 1 not string",
		[]interface{}{1},
		true,
	}, {
		"arg 2 not string",
		[]interface{}{"127.0.0.1/32", 1},
		true,
	}, {
		"arg 1 not CIDR",
		[]interface{}{"all the things"},
		true,
	}, {
		"one valid CIDR",
		[]interface{}{"1.2.3.4/32"},
		false,
	}, {
		"two valid CIDR",
		[]interface{}{"1.2.3.4/32", "1.2.3.4/32"},
		false,
	}, {
		"no net mask should default to /32",
		[]interface{}{"1.2.3.4"},
		false,
	}} {
		_, err := (&spec{}).Create(ti.args)
		if err == nil && ti.err || err != nil && !ti.err {
			t.Error(ti.msg, "failure case", err, ti.err)
		}
	}
}

func TestMatching(t *testing.T) {
	for _, ti := range []struct {
		msg     string
		args    []interface{}
		req     *http.Request
		matches bool
	}{{
		"happy case",
		[]interface{}{"127.0.0.1"},
		&http.Request{RemoteAddr: "127.0.0.1"},
		true,
	}, {
		"sad case",
		[]interface{}{"127.0.0.1"},
		&http.Request{RemoteAddr: "127.0.0.2"},
		false,
	}, {
		"should match on netmask",
		[]interface{}{"127.0.0.1/30"},
		&http.Request{RemoteAddr: "127.0.0.2"},
		true,
	}, {
		"should correctly handle netmask",
		[]interface{}{"127.0.0.0/31"},
		&http.Request{RemoteAddr: "127.0.0.2"},
		false,
	}, {
		"should correctly handle netmask",
		[]interface{}{"127.0.0.0/30"},
		&http.Request{RemoteAddr: "127.0.0.2"},
		true,
	}, {
		"should consider multiple masks",
		[]interface{}{"127.0.0.1", "8.8.8.8/24"},
		&http.Request{RemoteAddr: "8.8.8.127"},
		true,
	}, {
		"if available, should use X-Forwarded-For for matching",
		[]interface{}{"8.8.8.8"},
		&http.Request{RemoteAddr: "127.0.0.1", Header: http.Header{"X-Forwarded-For": []string{"8.8.8.8"}}},
		true,
	}, {
		"should use first X-Forwarded-For host (source instead of proxies) for matching",
		[]interface{}{"8.8.8.8"},
		&http.Request{RemoteAddr: "127.0.0.1", Header: http.Header{"X-Forwarded-For": []string{"8.8.8.8", "7.7.7.7", "6.6.6.6"}}},
		true,
	}} {
		pred, err := (&spec{}).Create(ti.args)
		if err != nil {
			t.Error("failed to create predicate", err)
		} else {
			matches := pred.Match(ti.req)
			if matches != ti.matches {
				t.Error(ti.msg, "failed to match as expected")
			}
		}
	}
}
