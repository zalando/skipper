package proxy_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/proxy/proxytest"
)

func testBackend(token string, code int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		w.Write([]byte(token))
	}))
}

func bindClient(p *proxytest.TestProxy) func(string, ...string) (string, bool) {
	return func(path string, expectedResponses ...string) (string, bool) {
	remaining:
		for range expectedResponses {
			rsp, err := http.Get(p.URL + path)
			if err != nil {
				return err.Error(), false
			}

			defer rsp.Body.Close()
			b, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				return err.Error(), false
			}

			res := strings.TrimSpace(string(b))
			for i := range expectedResponses {
				if res != expectedResponses[i] {
					continue
				}

				expectedResponses = append(expectedResponses[:i], expectedResponses[i+1:]...)
				continue remaining
			}

			return fmt.Sprintf("expect one of: %v, got: %s", expectedResponses, res), false
		}

		return "", true
	}
}

type closer interface {
	Close()
}

func setup() (*proxytest.TestProxy, []closer) {
	nonGrouped := testBackend("non-grouped", 200)
	nonGroupedFailing := newFailingBackend()
	groupA_BE1 := testBackend("group-A/BE-1", 200)
	groupA_BE2 := testBackend("group-A/BE-2", 200)
	groupB_BE1Failing := newFailingBackend()
	groupB_BE2 := testBackend("group-B/BE-2", 200)
	groupC_BE1 := testBackend("group-C/BE-1", 200)
	groupC_BE2Failing := newFailingBackend()
	groupD_BE1Failing := newFailingBackend()
	groupD_BE2Failing := newFailingBackend()

	cs := []closer{nonGrouped, nonGroupedFailing, groupA_BE1, groupA_BE2, groupB_BE1Failing, groupB_BE2, groupC_BE1, groupC_BE2Failing, groupD_BE1Failing, groupD_BE2Failing}

	const routesFmt = `
		nonGrouped: Path("/") -> "%s";
		nonGroupedFailing: Path("/fail") -> "%s";

		groupA: Path("/a") && LBGroup("group-a")
			-> lbDecide("group-a", 2)
			-> <loopback>;
		groupA_BE1: Path("/a") && LBMember("group-a", 0) -> "%s";
		groupA_BE2: Path("/a") && LBMember("group-a", 1) -> "%s";

		groupB: Path("/b") && LBGroup("group-b")
			-> lbDecide("group-b", 2)
			-> <loopback>;
		groupB_BE1_Failing: Path("/b") && LBMember("group-b", 0) -> "%s";
		groupB_BE2: Path("/b") && LBMember("group-b", 1) -> "%s";

		groupC: Path("/c") && LBGroup("group-c")
			-> lbDecide("group-c", 2)
			-> <loopback>;
		groupC_BE1: Path("/c") && LBMember("group-c", 0) -> "%s";
		groupC_BE2_Failing: Path("/c") && LBMember("group-c", 1) -> "%s";

		groupD: Path("/d") && LBGroup("group-d")
			-> lbDecide("group-d", 2)
			-> <loopback>;
		groupD_BE1_Failing: Path("/d") && LBMember("group-d", 0) -> "%s";
		groupD_BE2_Failing: Path("/d") && LBMember("group-d", 1) -> "%s";
	`

	routesDoc := fmt.Sprintf(
		routesFmt,
		nonGrouped.URL,
		nonGroupedFailing.url,
		groupA_BE1.URL,
		groupA_BE2.URL,
		groupB_BE1Failing.url,
		groupB_BE2.URL,
		groupC_BE1.URL,
		groupC_BE2Failing.url,
		groupD_BE1Failing.url,
		groupD_BE2Failing.url,
	)

	routes, err := eskip.Parse(routesDoc)
	if err != nil {
		log.Fatal(err)
	}

	p := proxytest.New(builtin.MakeRegistry(), routes...)

	groupB_BE1Failing.Close()
	groupC_BE2Failing.Close()
	groupD_BE1Failing.Close()
	groupD_BE2Failing.Close()

	return p, cs
}

func TestConnectionRefused(t *testing.T) {
	p, cs := setup()
	for _, c := range cs {
		defer c.Close()
	}
	defer p.Close()
	requestEndpoint := bindClient(p)

	t.Run("succeed and fail for all routes", func(t *testing.T) {
		if msg, ok := requestEndpoint("/", "non-grouped"); !ok {
			t.Errorf("failed to receive the right response for '/': %s", msg)
		}
		if msg, ok := requestEndpoint("/fail", ""); !ok {
			t.Errorf("failed to receive the right response for '/fail': %s", msg)
		}
		if msg, ok := requestEndpoint("/a", "group-A/BE-1", "group-A/BE-2"); !ok {
			t.Errorf("failed to receive the right response for '/a': %s", msg)
		}
		if msg, ok := requestEndpoint("/b", "group-B/BE-2"); !ok {
			t.Errorf("failed to receive the right response for '/b': %s", msg)
		}
		if msg, ok := requestEndpoint("/c", "group-C/BE-1"); !ok {
			t.Errorf("failed to receive the right response for '/c': %s", msg)
		}
		if msg, ok := requestEndpoint("/d", "Bad Gateway"); !ok {
			t.Errorf("failed to receive the right response for '/d': %s", msg)
		}
	})

	for _, ti := range []struct {
		msg          string
		path         string
		nTimes       int
		expectedCode int
	}{
		{
			msg:          "Groups with at least one working member should not fail on one call b",
			path:         "/b",
			nTimes:       1,
			expectedCode: 200,
		},
		{
			msg:          "Groups with at least one working member should not fail on one call c",
			path:         "/c",
			nTimes:       1,
			expectedCode: 200,
		},
		{
			msg:          "Groups with at least one working member should not fail on multiple call b",
			path:         "/b",
			nTimes:       150,
			expectedCode: 200,
		},
		{
			msg:          "Groups with at least one working member should not fail on multiple call c",
			path:         "/c",
			nTimes:       150,
			expectedCode: 200,
		},
		{
			msg:          "Groups with only failing members should  fail on multiple calls d",
			path:         "/d",
			nTimes:       150,
			expectedCode: http.StatusBadGateway,
		},
	} {
		t.Run(ti.msg, func(t *testing.T) {
			for i := 1; i <= ti.nTimes; i++ {
				rsp, err := http.Get(p.URL + ti.path)
				if err != nil {
					t.Errorf("loadbalanced request %d to %s should not fail: %v", i, ti.path, err)
				}
				if rsp.StatusCode != ti.expectedCode {
					t.Errorf("loadbalanced request %d to %s should have statuscode != %d: %d", i, ti.path, ti.expectedCode, rsp.StatusCode)
				}
			}
		})
	}
}

func BenchmarkConnectionRefusedA(b *testing.B) {
	idx := 2
	p, cs := setup()
	for i, c := range cs {
		if i != idx {
			defer c.Close()
		}
	}
	defer p.Close()
	expectedCode := 200
	path := "/a"

	for n := 0; n < b.N; n++ {
		if n == 200 {
			go cs[idx].Close()
		}
		rsp, err := http.Get(p.URL + path)
		if err != nil {
			b.Errorf("loadbalanced request %d to %s should not fail: %v", n, path, err)
		}
		if rsp.StatusCode != expectedCode {
			b.Errorf("loadbalanced request %d to %s should have statuscode != %d: %d", n, path, expectedCode, rsp.StatusCode)
		}
	}
}

func BenchmarkConnectionRefusedB(b *testing.B) {
	p, cs := setup()
	for _, c := range cs {
		defer c.Close()
	}
	defer p.Close()
	expectedCode := 200
	path := "/b"

	for n := 0; n < b.N; n++ {
		rsp, err := http.Get(p.URL + path)
		if err != nil {
			b.Errorf("loadbalanced request %d to %s should not fail: %v", n, path, err)
		}
		if rsp.StatusCode != expectedCode {
			b.Errorf("loadbalanced request %d to %s should have statuscode != %d: %d", n, path, expectedCode, rsp.StatusCode)
		}
	}
}

func BenchmarkConnectionRefusedC(b *testing.B) {
	p, cs := setup()
	for _, c := range cs {
		defer c.Close()
	}
	defer p.Close()
	expectedCode := 200
	path := "/c"

	for n := 0; n < b.N; n++ {
		rsp, err := http.Get(p.URL + path)
		if err != nil {
			b.Errorf("loadbalanced request %d to %s should not fail: %v", n, path, err)
		}
		if rsp.StatusCode != expectedCode {
			b.Errorf("loadbalanced request %d to %s should have statuscode != %d: %d", n, path, expectedCode, rsp.StatusCode)
		}
	}
}
