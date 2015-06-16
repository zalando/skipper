package settings

import "testing"
import "skipper/mock"
import "skipper/dispatch"
import "skipper/skipper"
import "net/http"
import "time"

func TestParseAndDispatchRawData(t *testing.T) {
	url1 := "https://www.zalando.de"
	data := map[string]interface{}{
		"backends": map[string]interface{}{"hello": url1},
		"frontends": []interface{}{
			map[string]interface{}{
				"route":      "Path(\"/hello\")",
				"backend-id": "hello"}}}

	dc := mock.MakeDataClient(data)
	mwr := &mock.MiddlewareRegistry{}
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
	if rt1.Backend().Url() != url1 || rt2.Backend().Url() != url1 {
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
	if rt1.Backend().Url() != url2 || rt2.Backend().Url() != url2 {
		t.Error("wrong url 2")
	}
}
