package settings

import (
	"net/http"
	"net/url"
	"skipper/dispatch"
	"skipper/mock"
	"skipper/skipper"
	"testing"
	"time"
)

func TestParseAndDispatchRawData(t *testing.T) {
	url1 := "https://www.zalando.de"
	data := map[string]interface{}{
		"backends": map[string]interface{}{"hello": url1},
		"frontends": map[string]interface{}{
			"hello": map[string]interface{}{
				"route":      "Path(\"/hello\")",
				"backend-id": "hello"}}}

	dc := mock.MakeDataClient(data)
	mwr := &mock.FilterRegistry{}
	d := dispatch.Make()
	s := MakeSource(dc, mwr, d)

	c1 := make(chan skipper.Settings)
	c2 := make(chan skipper.Settings)

	s.Subscribe(c1)
	s.Subscribe(c2)

	r, _ := http.NewRequest("GET", "http://localhost:9090/hello", nil)

	s1 := <-c1
	s2 := <-c2

	rt1, _ := s1.Route(r)
	rt2, _ := s2.Route(r)

	up1, _ := url.ParseRequestURI(url1)
	if rt1.Backend().Scheme() != up1.Scheme || rt1.Backend().Host() != up1.Host ||
		rt2.Backend().Scheme() != up1.Scheme || rt2.Backend().Host() != up1.Host {
		t.Error("wrong url 1")
	}

	url2 := "https://www.zalan.do"
	data["backends"].(map[string]interface{})["hello"] = url2
	dc.Feed(data)

	// let the new settings fan through
	time.Sleep(3 * time.Millisecond)

	s1 = <-c1
	s2 = <-c2

	rt1, _ = s1.Route(r)
	rt2, _ = s2.Route(r)
	up2, _ := url.ParseRequestURI(url2)
	if rt1.Backend().Scheme() != up2.Scheme || rt1.Backend().Host() != up2.Host ||
		rt2.Backend().Scheme() != up2.Scheme || rt2.Backend().Host() != up2.Host {
		t.Error("wrong url 2")
	}
}
