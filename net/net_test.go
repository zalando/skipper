package net

import (
	"net"
	"net/http"
	"reflect"
	"testing"
)

var netTests = []struct {
	input  string
	want   net.IP
	fwdHdr string
}{
	{"127.0.0.1", net.IPv4(127, 0, 0, 1), ""},
	{"1.2.3.4", net.IPv4(1, 2, 3, 4), ""},
	{"100.200.300.400", nil, ""},
	{"127.0.0.1:8080", net.IPv4(127, 0, 0, 1), ""},
	{"127.0.0.1", net.IPv4(172, 16, 0, 1), "172.16.0.1"},
	{"127.0.0.1", net.IPv4(127, 0, 0, 1), "invalid header"},
	{"127.0.0.1", net.IPv4(172, 16, 0, 1), "172.16.0.1, 1.2.3.4, 8.7.6.5"}, // X-Forwarded-For with proxies in it
	{"2001:4860:0:2001::68", net.ParseIP("2001:4860:0:2001::68"), ""},
	{"127.0.0.1", net.ParseIP("2001:4860:0:2001::68"), "2001:4860:0:2001::68"},
}

func TestRemoteHost(t *testing.T) {
	for _, test := range netTests {
		r := &http.Request{RemoteAddr: test.input, Header: make(http.Header)}
		if test.fwdHdr != "" {
			r.Header.Set("x-forwarded-for", test.fwdHdr)
		}
		got := RemoteHost(r)

		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("Unexpected IP address '%v'. Wanted '%v", got, test.want)
		}
	}
}

func BenchmarkRemoteHost(b *testing.B) {
	r := &http.Request{RemoteAddr: "1.2.3.4"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RemoteHost(r)
	}
}

func TestRemoteHostFromLast(t *testing.T) {
	var localNetTests = []struct {
		input  string
		want   net.IP
		fwdHdr string
	}{

		{"127.0.0.1", net.IPv4(8, 7, 6, 5), "172.16.0.1, 1.2.3.4, 8.7.6.5"},
	}
	for _, test := range localNetTests {
		r := &http.Request{RemoteAddr: test.input, Header: make(http.Header)}
		if test.fwdHdr != "" {
			r.Header.Set("x-forwarded-for", test.fwdHdr)
		}
		got := RemoteHostFromLast(r)

		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("Unexpected IP address '%v'. Wanted '%v", got, test.want)
		}
	}
}
