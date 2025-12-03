package traffic

import (
	"net/http"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/predicates/primitive"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"
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
		[]interface{}{.3, "testname", .2},
		predicate{},
		true,
	}, {
		"only chance",
		[]interface{}{.3},
		predicate{chance: .3},
		false,
	}, {
		"0.0 is allowed",
		[]interface{}{0.0},
		predicate{chance: 0},
		false,
	}, {
		"1.0 is allowed",
		[]interface{}{1.0},
		predicate{chance: 1},
		false,
	}, {
		"wrong chance, bigger than 1",
		[]interface{}{1.3},
		predicate{chance: 1.3},
		true,
	}, {
		"wrong chance, less than 0",
		[]interface{}{-0.3},
		predicate{chance: -0.3},
		true,
	}, {
		"you have 2 parameters but need to have 1 or 3 parameters",
		[]interface{}{.3, "foo"},
		predicate{chance: .3},
		true,
	}, {
		"chance and stickiness",
		[]interface{}{.3, "testname", "group"},
		predicate{chance: .3, trafficGroup: "group", trafficGroupCookie: "testname"},
		false,
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

func TestTrafficPredicateInRoutes(t *testing.T) {
	for _, tc := range []struct {
		msg        string
		routes     string
		expectedR1 float64
		expectedR2 float64
		expectedR3 float64
	}{{
		msg:        "no Traffic 100% match r1",
		routes:     `r1: * -> status(201) -> <shunt>`,
		expectedR1: 1,
	}, {
		msg:        "2 routes with 1 Traffic with 80% match r1",
		routes:     `r1: Traffic(0.8) -> status(201) -> <shunt>; r2: * -> status(202) -> <shunt>`,
		expectedR1: 0.8,
		expectedR2: 0.2,
	}, {
		msg:        "3 routes with 2 Traffic predicates with 50% match r1, 25% match the other",
		routes:     `r1: Traffic(0.5) && True() -> status(201) -> <shunt>; r2: Traffic(0.5) -> status(202) -> <shunt>; r3: * -> status(203) -> <shunt>`,
		expectedR1: 0.5,
		expectedR2: 0.25,
		expectedR3: 0.25,
	}} {
		t.Run(tc.msg, func(t *testing.T) {
			t.Parallel()
			var (
				goalR1 float64
				goalR2 float64
				goalR3 float64
			)
			N := 1000
			epsilonFactor := 0.1
			epsilon := float64(N) * epsilonFactor

			r := eskip.MustParse(tc.routes)

			p := proxytest.WithRoutingOptions(builtin.MakeRegistry(), routing.Options{
				Predicates: []routing.PredicateSpec{
					New(),
					primitive.NewTrue(),
				},
			}, r...)
			defer p.Close()

			req := func(u string) int {
				rsp, err := http.Get(u)
				if err != nil {
					t.Error(err)
					return -1
				}
				rsp.Body.Close()
				return rsp.StatusCode
			}

			r1 := 0.0
			r2 := 0.0
			r3 := 0.0
			for i := 0; i < N; i++ {
				n := req(p.URL)
				switch n {
				case 201:
					r1++
				case 202:
					r2++
				case 203:
					r3++
				default:
					t.Fatalf("Got %d", n)
				}
			}

			goalR1 = float64(N) * tc.expectedR1
			goalR2 = float64(N) * tc.expectedR2
			goalR3 = float64(N) * tc.expectedR3

			if goalR1-epsilon > r1 || goalR1+epsilon < r1 {
				t.Errorf("Failed to get the right traffic for r1 goal: %v - %v > %v || %v + %v < %v", goalR1, epsilon, r1, goalR1, epsilon, r1)
			}
			if goalR2-epsilon > r2 || goalR2+epsilon < r2 {
				t.Errorf("Failed to get the right traffic for r2 goal: %v - %v > %v || %v + %v < %v", goalR2, epsilon, r2, goalR2, epsilon, r2)
			}
			if goalR3-epsilon > r3 || goalR3+epsilon < r3 {
				t.Errorf("Failed to get the right traffic for r3 goal: %v - %v > %v || %v + %v < %v", goalR3, epsilon, r3, goalR3, epsilon, r3)
			}
		})
	}
}
