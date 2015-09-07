package settings

import (
	"github.com/zalando/skipper/mock"
	"github.com/zalando/skipper/skipper"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestParseAndDispatchRawData(t *testing.T) {
	url1 := "https://www.zalando.de"
	data := `hello: Path("/hello") -> "https://www.zalando.de"`

	dc := mock.MakeDataClient(data)
	fr := &mock.FilterRegistry{}
	s := MakeSource(dc, fr, false)

	c1 := make(chan skipper.Settings)
	c2 := make(chan skipper.Settings)

	s.Subscribe(c1)
	s.Subscribe(c2)

	r, _ := http.NewRequest("GET", "http://localhost:9090/hello", nil)

	// let the settings be populated:
	time.Sleep(15 * time.Millisecond)

	// TODO: this shouldn't be here
	// receive initial settings:
	<-c1
	<-c2

	s1 := <-c1
	s2 := <-c2

	rt1, _ := s1.Route(r)
	rt2, _ := s2.Route(r)

	up1, _ := url.ParseRequestURI(url1)

	if rt1 == nil || rt2 == nil {
		t.Error("invalid route", rt1 == nil, rt2 == nil)
		return
	}

	if rt1.Backend().Scheme() != up1.Scheme || rt1.Backend().Host() != up1.Host ||
		rt2.Backend().Scheme() != up1.Scheme || rt2.Backend().Host() != up1.Host {
		t.Error("wrong url 1")
	}

	data = `hello: Path("/hello") -> "https://www.zalan.do"`
	dc.Feed(data)

	// let the new settings fan through
	time.Sleep(3 * time.Millisecond)

	// TODO: this shouldn't be here
	// receive previous invalid settings:
	<-c1
	<-c2

	s1 = <-c1
	s2 = <-c2

	rt1, _ = s1.Route(r)
	rt2, _ = s2.Route(r)
	up2, _ := url.ParseRequestURI("https://www.zalan.do")
	if rt1.Backend().Scheme() != up2.Scheme || rt1.Backend().Host() != up2.Host ||
		rt2.Backend().Scheme() != up2.Scheme || rt2.Backend().Host() != up2.Host {
		t.Error("wrong url 2")
	}
}
