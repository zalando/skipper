package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/dataclients/routestring"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/routing"
)

func testPatch(t *testing.T, title string, f Flags, expectedStatus int) {
	t.Run(title, func(t *testing.T) {
		dc, err := routestring.New(`Path("/foo%2Fbar") -> status(200) -> <shunt>`)
		if err != nil {
			t.Fatal(err)
		}

		rt := routing.New(routing.Options{
			SignalFirstLoad: true,
			FilterRegistry:  builtin.MakeRegistry(),
			DataClients:     []routing.DataClient{dc},
		})
		defer rt.Close()

		p := WithParams(Params{Routing: rt, Flags: f})
		defer p.Close()

		s := httptest.NewServer(p)
		defer s.Close()

		<-rt.FirstLoad()

		rsp, err := http.Get(s.URL + "/foo%2Fbar")
		if err != nil {
			t.Fatal(err)
		}

		defer rsp.Body.Close()

		if rsp.StatusCode != expectedStatus {
			t.Error()
		}
	})
}

func TestPathPatch(t *testing.T) {
	testPatch(t, "not patched", FlagsNone, http.StatusNotFound)
	testPatch(t, "patched", PatchPath, http.StatusOK)
}
