package net

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type tc[T any] struct {
	location string
	in       T
}

// https://github.com/golang/go/issues/52751
func testCase[T any](in T) tc[T] {
	_, file, line, _ := runtime.Caller(1)
	location := fmt.Sprintf("%s:%d", filepath.Base(file), line)
	return tc[T]{location: location, in: in}
}

func (tc *tc[T]) logLocation(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		t.Helper()
		if t.Failed() {
			t.Logf("Test case location: %s", tc.location)
		}
	})
}

func TestRemoteAddr(t *testing.T) {
	for _, tt := range []struct {
		name   string
		input  string
		want   netip.Addr
		fwdHdr string
	}{
		{"no header1", "127.0.0.1", netip.MustParseAddr("127.0.0.1"), ""},
		{"no header2", "1.2.3.4", netip.MustParseAddr("1.2.3.4"), ""},
		{"no header3", "100.200.300.400", netip.Addr{}, ""},
		{"no header4", "127.0.0.1:8080", netip.MustParseAddr("127.0.0.1"), ""},
		{"single header1", "127.0.0.1", netip.MustParseAddr("172.16.0.1"), "172.16.0.1"},
		{"invalid header", "127.0.0.1", netip.MustParseAddr("127.0.0.1"), "invalid header"},
		{"multiple header1", "127.0.0.1", netip.MustParseAddr("172.16.0.1"), "172.16.0.1, 1.2.3.4, 8.7.6.5"}, // X-Forwarded-For with proxies in it
		{"no header5", "2001:4860:0:2001::68", netip.MustParseAddr("2001:4860:0:2001::68"), ""},
		{"single header2", "127.0.0.1", netip.MustParseAddr("2001:4860:0:2001::68"), "2001:4860:0:2001::68"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: tt.input, Header: make(http.Header)}
			if tt.fwdHdr != "" {
				r.Header.Set("x-forwarded-for", tt.fwdHdr)
			}
			got := RemoteAddr(r)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Unexpected IP address '%v'. Wanted '%v", got, tt.want)
			}
		})
	}
}
func TestRemoteHost(t *testing.T) {
	for _, tt := range []struct {
		name   string
		input  string
		want   net.IP
		fwdHdr string
	}{
		{"no header1", "127.0.0.1", net.IPv4(127, 0, 0, 1), ""},
		{"no header2", "1.2.3.4", net.IPv4(1, 2, 3, 4), ""},
		{"no header3", "100.200.300.400", nil, ""},
		{"no header4", "127.0.0.1:8080", net.IPv4(127, 0, 0, 1), ""},
		{"single header1", "127.0.0.1", net.IPv4(172, 16, 0, 1), "172.16.0.1"},
		{"invalid header", "127.0.0.1", net.IPv4(127, 0, 0, 1), "invalid header"},
		{"multiple header1", "127.0.0.1", net.IPv4(172, 16, 0, 1), "172.16.0.1, 1.2.3.4, 8.7.6.5"}, // X-Forwarded-For with proxies in it
		{"no header5", "2001:4860:0:2001::68", net.ParseIP("2001:4860:0:2001::68"), ""},
		{"single header2", "127.0.0.1", net.ParseIP("2001:4860:0:2001::68"), "2001:4860:0:2001::68"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: tt.input, Header: make(http.Header)}
			if tt.fwdHdr != "" {
				r.Header.Set("x-forwarded-for", tt.fwdHdr)
			}
			got := RemoteHost(r)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Unexpected IP address '%v'. Wanted '%v", got, tt.want)
			}
		})
	}
}

func BenchmarkRemoteHost(b *testing.B) {
	r := &http.Request{RemoteAddr: "1.2.3.4"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RemoteHost(r)
	}
}

func TestRemoteAddrFromLast(t *testing.T) {
	for _, tt := range []struct {
		name   string
		input  string
		want   netip.Addr
		fwdHdr []string
	}{
		{"no header", "127.0.0.1", netip.MustParseAddr("127.0.0.1"), []string{}},
		{"no header2", "1.2.3.4", netip.MustParseAddr("1.2.3.4"), []string{}},
		{"no header3", "100.200.300.400", netip.Addr{}, []string{}},
		{"no header4", "127.0.0.1:8080", netip.MustParseAddr("127.0.0.1"), []string{}},
		{"single header", "127.0.0.1", netip.MustParseAddr("172.16.0.1"), []string{"172.16.0.1"}},
		{"invalid  header", "127.0.0.1", netip.MustParseAddr("127.0.0.1"), []string{"invalid header"}},
		{"invalid and remoteIp", "invalid-ip", netip.Addr{}, []string{"invalid, header"}},
		{"multiple entries", "127.0.0.1", netip.MustParseAddr("8.7.6.5"), []string{"172.16.0.1", "1.2.3.4", "8.7.6.5"}},
		{"2 entries", "127.0.0.1", netip.MustParseAddr("8.7.6.5"), []string{"1.2.3.4", "8.7.6.5"}},
		{"single header2", "127.0.0.1", netip.MustParseAddr("8.7.6.5"), []string{"8.7.6.5"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: tt.input, Header: make(http.Header)}
			s := strings.Join(tt.fwdHdr, ", ")
			r.Header.Set("x-forwarded-for", s)
			got := RemoteAddrFromLast(r)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Unexpected IP address '%v'. Wanted '%v", got, tt.want)
			}
		})
	}
}

func TestRemoteHostFromLast(t *testing.T) {
	for _, tt := range []struct {
		name   string
		input  string
		want   net.IP
		fwdHdr []string
	}{
		{"no header", "127.0.0.1", net.IPv4(127, 0, 0, 1), []string{}},
		{"no header2", "1.2.3.4", net.IPv4(1, 2, 3, 4), []string{}},
		{"no header3", "100.200.300.400", nil, []string{}},
		{"no header4", "127.0.0.1:8080", net.IPv4(127, 0, 0, 1), []string{}},
		{"single header", "127.0.0.1", net.IPv4(172, 16, 0, 1), []string{"172.16.0.1"}},
		{"invalid  header", "127.0.0.1", net.IPv4(127, 0, 0, 1), []string{"invalid header"}},
		{"multiple entries", "127.0.0.1", net.IPv4(8, 7, 6, 5), []string{"172.16.0.1", "1.2.3.4", "8.7.6.5"}},
		{"2 entries", "127.0.0.1", net.IPv4(8, 7, 6, 5), []string{"1.2.3.4", "8.7.6.5"}},
		{"single header2", "127.0.0.1", net.IPv4(8, 7, 6, 5), []string{"8.7.6.5"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: tt.input, Header: make(http.Header)}
			s := strings.Join(tt.fwdHdr, ", ")
			r.Header.Set("x-forwarded-for", s)
			got := RemoteHostFromLast(r)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Unexpected IP address '%v'. Wanted '%v", got, tt.want)
			}
		})
	}
}

func BenchmarkRemoteHostFromLast(b *testing.B) {
	r := &http.Request{RemoteAddr: "1.2.3.4"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RemoteHostFromLast(r)
	}
}

func TestParseIPCIDRs(t *testing.T) {
	for _, tt := range []struct {
		input   []string
		wantErr bool
	}{
		{[]string{"::"}, true},
		{[]string{"f::"}, false},
		{[]string{"::1"}, false},
		{[]string{"::1/8"}, false},
		{[]string{"1.2.3.4.5"}, true},
		{[]string{"1.2.3.4/"}, true},
		{[]string{"1.2.3.4/245"}, true},
		{[]string{"whatever"}, true},
		{[]string{"1.2.3.4/24", "whatever"}, true},
		{[]string{"1.2.3.4"}, false},
		{[]string{"1.2.3.4/16"}, false},
		{[]string{"1.2.3.4", "foo", "1.2.3.4/16"}, true},
	} {
		t.Run(strings.Join(tt.input, ","), func(t *testing.T) {
			_, err := ParseIPCIDRs(tt.input)
			if tt.wantErr && err == nil {
				t.Logf("ParseIPCIDRs: %v", tt.input)
				t.Errorf("parse error expected")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("parse error unexpected: %v", err)
			}
		})
	}
}

func TestIPNetsParse(t *testing.T) {
	for _, tt := range []struct {
		input []string
	}{
		{[]string{"1.2.3.4.5"}},
		{[]string{"1.2.3.4/"}},
		{[]string{"1.2.3.4/245"}},
		{[]string{"whatever"}},
		{[]string{"1.2.3.4/24", "whatever"}},
	} {
		t.Run(strings.Join(tt.input, ","), func(t *testing.T) {
			_, err := ParseCIDRs(tt.input)
			t.Logf("ParseCIDRs: %v", err)
			if err == nil {
				t.Errorf("parse error expected")
			}
		})
	}
}

func TestIPNetsContain(t *testing.T) {
	for _, tt := range []struct {
		input []string
		ip    net.IP
	}{
		{[]string{"1.2.3.4/24"}, net.IPv4(1, 2, 3, 4)},
		{[]string{"1.2.3.4/24", "5.6.7.8"}, net.IPv4(5, 6, 7, 8)},
		{[]string{"1.2.3.4/24", "5.6.7.8", "2001:db8::/32"}, net.ParseIP("2001:db8::aa")},
	} {
		t.Run(strings.Join(tt.input, ","), func(t *testing.T) {
			nets, err := ParseCIDRs(tt.input)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !nets.Contain(tt.ip) {
				t.Errorf("nets %v expected to contain %v", nets, tt.ip)
			}
		})
	}
}

func TestIPNetsDoNotContain(t *testing.T) {
	for _, tt := range []struct {
		input []string
		ip    net.IP
	}{
		{[]string{}, net.IPv4(4, 3, 2, 1)},
		{[]string{"1.2.3.4/24"}, net.IPv4(4, 3, 2, 1)},
		{[]string{"1.2.3.4/24", "5.6.7.8"}, net.IPv4(4, 3, 2, 1)},
		{[]string{"1.2.3.4/24", "5.6.7.8", "2001:db8::/32"}, net.IPv4(4, 3, 2, 1)},
	} {
		t.Run(strings.Join(tt.input, ","), func(t *testing.T) {
			nets, err := ParseCIDRs(tt.input)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if nets.Contain(tt.ip) {
				t.Errorf("nets %v expected to contain %v", nets, tt.ip)
			}
		})
	}
}

type TestSchemeHostItem struct {
	input  string
	scheme string
	host   string
	err    string
}

func TestSchemeHost(t *testing.T) {
	for _, ti := range []tc[TestSchemeHostItem]{
		testCase(TestSchemeHostItem{
			input:  "http://example.com",
			scheme: "http",
			host:   "example.com:80",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "http://example.com:80",
			scheme: "http",
			host:   "example.com:80",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "http://example.com:8080",
			scheme: "http",
			host:   "example.com:8080",
			err:    "",
		}),

		testCase(TestSchemeHostItem{
			input:  "https://example.com",
			scheme: "https",
			host:   "example.com:443",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "https://example.com:443",
			scheme: "https",
			host:   "example.com:443",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "https://example.com:8080",
			scheme: "https",
			host:   "example.com:8080",
			err:    "",
		}),

		testCase(TestSchemeHostItem{
			input:  "fastcgi://example.com",
			scheme: "fastcgi",
			host:   "example.com",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "fastcgi://example.com:9000",
			scheme: "fastcgi",
			host:   "example.com:9000",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "fastcgi://example.com:8080",
			scheme: "fastcgi",
			host:   "example.com:8080",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "fastcgi://foo/bar",
			scheme: "fastcgi",
			host:   "foo",
			err:    "",
		}),

		testCase(TestSchemeHostItem{
			input:  "postgres://example.com",
			scheme: "postgres",
			host:   "example.com",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "postgres://example.com:5432",
			scheme: "postgres",
			host:   "example.com:5432",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "postgresql://example.com",
			scheme: "postgresql",
			host:   "example.com",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "postgresql://example.com:5432",
			scheme: "postgresql",
			host:   "example.com:5432",
			err:    "",
		}),

		testCase(TestSchemeHostItem{
			input:  "someprotocol://example.com",
			scheme: "someprotocol",
			host:   "example.com",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "someprotocol://example.com:12345",
			scheme: "someprotocol",
			host:   "example.com:12345",
			err:    "",
		}),

		testCase(TestSchemeHostItem{
			input:  "example.com",
			scheme: "",
			host:   "",
			err:    `parse "example.com": invalid URI for request`,
		}),
		testCase(TestSchemeHostItem{
			input:  "example.com/",
			scheme: "",
			host:   "",
			err:    `parse "example.com/": invalid URI for request`,
		}),
		testCase(TestSchemeHostItem{
			input:  "example.com:80",
			scheme: "",
			host:   "",
			err:    `parse "example.com:80": missing host`,
		}),

		testCase(TestSchemeHostItem{
			input:  "hTTP://exAMPLe.com",
			scheme: "http",
			host:   "example.com:80",
			err:    "",
		}),

		testCase(TestSchemeHostItem{
			input:  "http://example.com/foo/bar",
			scheme: "http",
			host:   "example.com:80",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "http://example.com:80/foo/bar",
			scheme: "http",
			host:   "example.com:80",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "http://example.com:8080/foo/bar",
			scheme: "http",
			host:   "example.com:8080",
			err:    "",
		}),

		testCase(TestSchemeHostItem{
			input:  "http://example.com?foo=bar",
			scheme: "http",
			host:   "example.com:80",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "http://example.com:80?foo=bar",
			scheme: "http",
			host:   "example.com:80",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "http://example.com:8080?foo=bar",
			scheme: "http",
			host:   "example.com:8080",
			err:    "",
		}),

		testCase(TestSchemeHostItem{
			input:  "http://192.168.0.1",
			scheme: "http",
			host:   "192.168.0.1:80",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "http://192.168.0.1:80",
			scheme: "http",
			host:   "192.168.0.1:80",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "http://192.168.0.1:8080",
			scheme: "http",
			host:   "192.168.0.1:8080",
			err:    "",
		}),

		testCase(TestSchemeHostItem{
			input:  "http://[2001:db8:3333:4444:5555:6666:7777:8888]",
			scheme: "http",
			host:   "[2001:db8:3333:4444:5555:6666:7777:8888]:80",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "http://[2001:db8:3333:4444:5555:6666:7777:8888]:80",
			scheme: "http",
			host:   "[2001:db8:3333:4444:5555:6666:7777:8888]:80",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "http://[2001:db8:3333:4444:5555:6666:7777:8888]:8080",
			scheme: "http",
			host:   "[2001:db8:3333:4444:5555:6666:7777:8888]:8080",
			err:    "",
		}),

		testCase(TestSchemeHostItem{
			input:  "fastcgi://192.168.0.1",
			scheme: "fastcgi",
			host:   "192.168.0.1",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "fastcgi://192.168.0.1:9000",
			scheme: "fastcgi",
			host:   "192.168.0.1:9000",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "fastcgi://[2001:db8:3333:4444:5555:6666:7777:8888]",
			scheme: "fastcgi",
			host:   "2001:db8:3333:4444:5555:6666:7777:8888",
			err:    "",
		}),
		testCase(TestSchemeHostItem{
			input:  "fastcgi://[2001:db8:3333:4444:5555:6666:7777:8888]:9000",
			scheme: "fastcgi",
			host:   "[2001:db8:3333:4444:5555:6666:7777:8888]:9000",
			err:    "",
		}),

		testCase(TestSchemeHostItem{
			input:  "/foo",
			scheme: "",
			host:   "",
			err:    `parse "/foo": missing scheme`,
		}),
	} {
		t.Run(ti.in.input, func(t *testing.T) {
			ti.logLocation(t)

			scheme, host, err := SchemeHost(ti.in.input)
			if ti.in.err != "" {
				assert.EqualError(t, err, ti.in.err)
			} else {
				if assert.NoError(t, err) {
					assert.Equal(t, ti.in.scheme, scheme)
					assert.Equal(t, ti.in.host, host)
				}
			}
		})
	}
}
