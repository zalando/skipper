package source

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/predicates"
)

func TestName(t *testing.T) {
	if s := New().Name(); s != predicates.SourceName {
		t.Fatalf("Failed to get Name %s, got %s", predicates.SourceName, s)
	}
	if s := NewFromLast().Name(); s != predicates.SourceFromLastName {
		t.Fatalf("Failed to get Name %s, got %s", predicates.SourceFromLastName, s)
	}
	if s := NewClientIP().Name(); s != predicates.ClientIPName {
		t.Fatalf("Failed to get Name %s, got %s", predicates.ClientIPName, s)
	}
}

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
		"arg 1 not netmask",
		[]interface{}{"all the things"},
		true,
	}, {
		"one valid netmask",
		[]interface{}{"1.2.3.4/32"},
		false,
	}, {
		"two valid netmasks",
		[]interface{}{"1.2.3.4/32", "1.2.3.4/32"},
		false,
	}, {
		"no net mask should default to /32",
		[]interface{}{"1.2.3.4"},
		false,
	}, {
		"should handle IPv6 addresses",
		[]interface{}{"C0:FF::EE"},
		false,
	}, {
		"should handle IPv6 with mask",
		[]interface{}{"C0:FF::EE/32"},
		false,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			_, err := (New()).Create(ti.args)
			if err == nil && ti.err || err != nil && !ti.err {
				t.Error(ti.msg, "failure case", err, ti.err)
			}
			_, err = (NewFromLast()).Create(ti.args)
			if err == nil && ti.err || err != nil && !ti.err {
				t.Error(ti.msg, "failure case", err, ti.err)
			}
			_, err = (NewClientIP()).Create(ti.args)
			if err == nil && ti.err || err != nil && !ti.err {
				t.Error(ti.msg, "failure case", err, ti.err)
			}
		})
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
		&http.Request{RemoteAddr: "127.0.0.1", Header: http.Header{"X-Forwarded-For": []string{"8.8.8.8, 7.7.7.7, 6.6.6.6"}}},
		true,
	}, {
		"should work for IPv6",
		[]interface{}{"C0:FF::EE"},
		&http.Request{RemoteAddr: "[C0:FF::EE]:5123"},
		true,
	}, {
		"should work for IPv6 with mask - pass",
		[]interface{}{"C0:FF::EE/127"},
		&http.Request{RemoteAddr: "[C0:FF::EF]:5123"},
		true,
	}, {
		"should work for IPv6 with mask - reject",
		[]interface{}{"C0:FF::EE/127"},
		&http.Request{RemoteAddr: "[C0:FF::EC]:5123"},
		false,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			pred, err := (&spec{}).Create(ti.args)
			if err != nil {
				t.Error("failed to create predicate", err)
			} else {
				matches := pred.Match(ti.req)
				if matches != ti.matches {
					t.Error(ti.msg, "failed to match as expected")
				}
			}
		})
	}
}

func TestMatchingFromLast(t *testing.T) {
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
		[]interface{}{"6.6.6.6"},
		&http.Request{RemoteAddr: "127.0.0.1", Header: http.Header{"X-Forwarded-For": []string{"8.8.8.8, 7.7.7.7, 6.6.6.6"}}},
		true,
	}, {
		"should work for IPv6",
		[]interface{}{"C0:FF::EE"},
		&http.Request{RemoteAddr: "[C0:FF::EE]:4123"},
		true,
	}, {
		"should work for IPv6 with mask - pass",
		[]interface{}{"C0:FF::EE/127"},
		&http.Request{RemoteAddr: "[C0:FF::EF]:4123"},
		true,
	}, {
		"should work for IPv6 with mask - reject",
		[]interface{}{"C0:FF::EE/127"},
		&http.Request{RemoteAddr: "[C0:FF::EC]:4123"},
		false,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			pred, err := (&spec{typ: sourceFromLast}).Create(ti.args)
			if err != nil {
				t.Error("failed to create predicate", err)
			} else {
				matches := pred.Match(ti.req)
				if matches != ti.matches {
					t.Error(ti.msg, "failed to match from last as expected")
				}
			}
		})
	}
}

func TestMatchingClientIP(t *testing.T) {
	for _, ti := range []struct {
		msg     string
		args    []interface{}
		req     *http.Request
		matches bool
	}{{
		"happy case",
		[]interface{}{"127.0.0.1"},
		&http.Request{RemoteAddr: "127.0.0.1:1234"},
		true,
	}, {
		"sad case",
		[]interface{}{"127.0.0.1"},
		&http.Request{RemoteAddr: "127.0.0.2:51234"},
		false,
	}, {
		"should match on netmask",
		[]interface{}{"127.0.0.1/30"},
		&http.Request{RemoteAddr: "127.0.0.2:1234"},
		true,
	}, {
		"should correctly handle netmask",
		[]interface{}{"127.0.0.0/31"},
		&http.Request{RemoteAddr: "127.0.0.2:1234"},
		false,
	}, {
		"should correctly handle netmask",
		[]interface{}{"127.0.0.0/30"},
		&http.Request{RemoteAddr: "127.0.0.2:1234"},
		true,
	}, {
		"should consider multiple masks",
		[]interface{}{"127.0.0.1", "8.8.8.8/24"},
		&http.Request{RemoteAddr: "8.8.8.127:1234"},
		true,
	}, {
		"if available, should not use X-Forwarded-For for matching,match",
		[]interface{}{"127.0.0.1"},
		&http.Request{RemoteAddr: "127.0.0.1:1234", Header: http.Header{"X-Forwarded-For": []string{"8.8.8.8"}}},
		true,
	}, {
		"if available, should not use X-Forwarded-For for matching, no match",
		[]interface{}{"8.8.8.8"},
		&http.Request{RemoteAddr: "127.0.0.1:1234", Header: http.Header{"X-Forwarded-For": []string{"8.8.8.8"}}},
		false,
	}, {
		"should work for IPv6",
		[]interface{}{"C0:FF::EE"},
		&http.Request{RemoteAddr: "[C0:FF::EE]:1234"},
		true,
	}, {
		"should work for IPv6 with mask - pass",
		[]interface{}{"C0:FF::EE/127"},
		&http.Request{RemoteAddr: "[C0:FF::EF]:1234"},
		true,
	}, {
		"should work for IPv6 with mask - reject",
		[]interface{}{"C0:FF::EE/127"},
		&http.Request{RemoteAddr: "[C0:FF::EC]:1234"},
		false,
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			pred, err := (&spec{typ: clientIP}).Create(ti.args)
			if err != nil {
				t.Error("failed to create predicate", err)
			} else {
				matches := pred.Match(ti.req)
				if matches != ti.matches {
					t.Error(ti.msg, "failed to match as expected")
				}
			}
		})
	}
}
