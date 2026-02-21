package source

import (
	"fmt"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
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
		args []any
		err  bool
	}{{
		"no args",
		nil,
		true,
	}, {
		"arg 1 not string",
		[]any{1},
		true,
	}, {
		"arg 2 not string",
		[]any{"127.0.0.1/32", 1},
		true,
	}, {
		"one invalid IP",
		[]any{"1.2.3.4.5/16", "1000.2.3.4"},
		true,
	}, {
		"arg 1 not netmask, silently ignored",
		[]any{"all the things"},
		true,
	}, {
		"one valid netmask",
		[]any{"1.2.3.4/32"},
		false,
	}, {
		"two valid netmasks",
		[]any{"1.2.3.4/32", "1.2.3.4/32"},
		false,
	}, {
		"no net mask should default to /32",
		[]any{"1.2.3.4"},
		false,
	}, {
		"should handle IPv6 addresses",
		[]any{"C0:FF::EE"},
		false,
	}, {
		"should handle IPv6 with mask",
		[]any{"C0:FF::EE/32"},
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
		args    []any
		req     *http.Request
		matches bool
	}{{
		"happy case",
		[]any{"127.0.0.1"},
		&http.Request{RemoteAddr: "127.0.0.1"},
		true,
	}, {
		"sad case",
		[]any{"127.0.0.1"},
		&http.Request{RemoteAddr: "127.0.0.2"},
		false,
	}, {
		"should match on netmask",
		[]any{"127.0.0.1/30"},
		&http.Request{RemoteAddr: "127.0.0.2"},
		true,
	}, {
		"should correctly handle netmask",
		[]any{"127.0.0.0/31"},
		&http.Request{RemoteAddr: "127.0.0.2"},
		false,
	}, {
		"should correctly handle netmask",
		[]any{"127.0.0.0/30"},
		&http.Request{RemoteAddr: "127.0.0.2"},
		true,
	}, {
		"should consider multiple masks",
		[]any{"127.0.0.1", "8.8.8.8/24"},
		&http.Request{RemoteAddr: "8.8.8.127"},
		true,
	}, {
		"if available, should use X-Forwarded-For for matching",
		[]any{"8.8.8.8"},
		&http.Request{RemoteAddr: "127.0.0.1", Header: http.Header{"X-Forwarded-For": []string{"8.8.8.8"}}},
		true,
	}, {
		"should use first X-Forwarded-For host (source instead of proxies) for matching",
		[]any{"8.8.8.8"},
		&http.Request{RemoteAddr: "127.0.0.1", Header: http.Header{"X-Forwarded-For": []string{"8.8.8.8, 7.7.7.7, 6.6.6.6"}}},
		true,
	}, {
		"should work for IPv6",
		[]any{"C0:FF::EE"},
		&http.Request{RemoteAddr: "[C0:FF::EE]:5123"},
		true,
	}, {
		"should work for IPv6 with mask - pass",
		[]any{"C0:FF::EE/127"},
		&http.Request{RemoteAddr: "[C0:FF::EF]:5123"},
		true,
	}, {
		"should work for IPv6 with mask - reject",
		[]any{"C0:FF::EE/127"},
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
		args    []any
		req     *http.Request
		matches bool
	}{{
		"happy case",
		[]any{"127.0.0.1"},
		&http.Request{RemoteAddr: "127.0.0.1"},
		true,
	}, {
		"sad case",
		[]any{"127.0.0.1"},
		&http.Request{RemoteAddr: "127.0.0.2"},
		false,
	}, {
		"should match on netmask",
		[]any{"127.0.0.1/30"},
		&http.Request{RemoteAddr: "127.0.0.2"},
		true,
	}, {
		"should correctly handle netmask",
		[]any{"127.0.0.0/31"},
		&http.Request{RemoteAddr: "127.0.0.2"},
		false,
	}, {
		"should correctly handle netmask",
		[]any{"127.0.0.0/30"},
		&http.Request{RemoteAddr: "127.0.0.2"},
		true,
	}, {
		"should consider multiple masks",
		[]any{"127.0.0.1", "8.8.8.8/24"},
		&http.Request{RemoteAddr: "8.8.8.127"},
		true,
	}, {
		"if available, should use X-Forwarded-For for matching",
		[]any{"8.8.8.8"},
		&http.Request{RemoteAddr: "127.0.0.1", Header: http.Header{"X-Forwarded-For": []string{"8.8.8.8"}}},
		true,
	}, {
		"should use first X-Forwarded-For host (source instead of proxies) for matching",
		[]any{"6.6.6.6"},
		&http.Request{RemoteAddr: "127.0.0.1", Header: http.Header{"X-Forwarded-For": []string{"8.8.8.8, 7.7.7.7, 6.6.6.6"}}},
		true,
	}, {
		"should work for IPv6",
		[]any{"C0:FF::EE"},
		&http.Request{RemoteAddr: "[C0:FF::EE]:4123"},
		true,
	}, {
		"should work for IPv6 with mask - pass",
		[]any{"C0:FF::EE/127"},
		&http.Request{RemoteAddr: "[C0:FF::EF]:4123"},
		true,
	}, {
		"should work for IPv6 with mask - reject",
		[]any{"C0:FF::EE/127"},
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
		args    []any
		req     *http.Request
		matches bool
	}{{
		"happy case",
		[]any{"127.0.0.1"},
		&http.Request{RemoteAddr: "127.0.0.1:1234"},
		true,
	}, {
		"sad case",
		[]any{"127.0.0.1"},
		&http.Request{RemoteAddr: "127.0.0.2:51234"},
		false,
	}, {
		"should match on netmask",
		[]any{"127.0.0.1/30"},
		&http.Request{RemoteAddr: "127.0.0.2:1234"},
		true,
	}, {
		"should correctly handle netmask",
		[]any{"127.0.0.0/31"},
		&http.Request{RemoteAddr: "127.0.0.2:1234"},
		false,
	}, {
		"should correctly handle netmask",
		[]any{"127.0.0.0/30"},
		&http.Request{RemoteAddr: "127.0.0.2:1234"},
		true,
	}, {
		"should consider multiple masks",
		[]any{"127.0.0.1", "8.8.8.8/24"},
		&http.Request{RemoteAddr: "8.8.8.127:1234"},
		true,
	}, {
		"if available, should not use X-Forwarded-For for matching,match",
		[]any{"127.0.0.1"},
		&http.Request{RemoteAddr: "127.0.0.1:1234", Header: http.Header{"X-Forwarded-For": []string{"8.8.8.8"}}},
		true,
	}, {
		"if available, should not use X-Forwarded-For for matching, no match",
		[]any{"8.8.8.8"},
		&http.Request{RemoteAddr: "127.0.0.1:1234", Header: http.Header{"X-Forwarded-For": []string{"8.8.8.8"}}},
		false,
	}, {
		"should work for IPv6",
		[]any{"C0:FF::EE"},
		&http.Request{RemoteAddr: "[C0:FF::EE]:1234"},
		true,
	}, {
		"should work for IPv6 with mask - pass",
		[]any{"C0:FF::EE/127"},
		&http.Request{RemoteAddr: "[C0:FF::EF]:1234"},
		true,
	}, {
		"should work for IPv6 with mask - reject",
		[]any{"C0:FF::EE/127"},
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

var random = rand.New(rand.NewSource(time.Now().UnixNano()))

func generateIPCidr() string {
	if m := random.Int(); m%10 == 0 {
		n := random.Intn(16) + 16 // /16 .. /32
		return fmt.Sprintf("%d.%d.%d.%d/%d", random.Intn(256), random.Intn(256), random.Intn(256), random.Intn(256), n)
	} else if m%2 == 0 {
		n := random.Intn(16) + 48 // /16 .. /32
		return fmt.Sprintf("%02x:%02x::af:%02x:%02x/%d", random.Intn(16), random.Intn(16), random.Intn(16), random.Intn(16), n)
	}
	return fmt.Sprintf("%d.%d.%d.%d", random.Intn(256), random.Intn(256), random.Intn(256), random.Intn(256))
}

func generateIPCidrStrings(n int) []string {
	a := make([]string, 0)
	for range n {
		a = append(a, generateIPCidr())
	}
	return a
}

func BenchmarkClientIP10(b *testing.B) {
	benchSource(b, 10, NewClientIP())
}
func BenchmarkClientIP100(b *testing.B) {
	benchSource(b, 100, NewClientIP())
}
func BenchmarkClientIP1k(b *testing.B) {
	benchSource(b, 1_000, NewClientIP())
}
func BenchmarkClientIP10k(b *testing.B) {
	benchSource(b, 10_000, NewClientIP())
}
func BenchmarkClientIP100k(b *testing.B) {
	benchSource(b, 100_000, NewClientIP())
}

func BenchmarkSource10(b *testing.B) {
	benchSource(b, 10, New())
}
func BenchmarkSource100(b *testing.B) {
	benchSource(b, 100, New())
}
func BenchmarkSource1k(b *testing.B) {
	benchSource(b, 1_000, New())
}
func BenchmarkSource10k(b *testing.B) {
	benchSource(b, 10_000, New())
}
func BenchmarkSource100k(b *testing.B) {
	benchSource(b, 100_000, New())
}

func BenchmarkSourceFromLast10(b *testing.B) {
	benchSource(b, 10, NewFromLast())
}
func BenchmarkSourceFromLast100(b *testing.B) {
	benchSource(b, 100, NewFromLast())
}
func BenchmarkSourceFromLast1k(b *testing.B) {
	benchSource(b, 1_000, NewFromLast())
}
func BenchmarkSourceFromLast10k(b *testing.B) {
	benchSource(b, 10_000, NewFromLast())
}
func BenchmarkSourceFromLast100k(b *testing.B) {
	benchSource(b, 100_000, NewFromLast())
}

func stringsToArgs(a []string) []any {
	res := make([]any, 0, len(a))
	for _, s := range a {
		var e any = s
		res = append(res, e)
	}
	return res
}

func benchSource(b *testing.B, k int, spec routing.PredicateSpec) {
	target := "195.5.1.21"
	xff := "195.5.1.23, 195.5.1.24"
	a := generateIPCidrStrings(k)
	args := stringsToArgs(a)

	pred, err := spec.Create(args)
	if err != nil {
		b.Fatalf("Failed to create %s with args '%v': %v", spec.Name(), args, err)
	}
	req := &http.Request{
		RemoteAddr: target + ":12345",
		Header:     http.Header{},
	}
	req.Header.Set("X-Forwarded-For", xff)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		pred.Match(req)
	}
}
