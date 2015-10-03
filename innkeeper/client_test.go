package innkeeper

import (
	"encoding/json"
	"errors"
	"github.com/zalando/skipper/eskip"
	"net/http"
	"net/http/httptest"
	"path"
	"sort"
	"strings"
	"testing"
	"time"
)

const testAuthenticationToken = "test token"

type autoAuth bool

func (aa autoAuth) Token() (string, error) {
	if aa {
		return testAuthenticationToken, nil
	}

	return "", errors.New(string(authErrorAuthentication))
}

type innkeeperHandler struct{ data []*routeData }

func (h *innkeeperHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(authHeaderName) != testAuthenticationToken {
		w.WriteHeader(http.StatusUnauthorized)
		enc := json.NewEncoder(w)

		// ignoring error
		enc.Encode(&apiError{ErrorType: string(authErrorPermission)})

		return
	}

	var responseData []*routeData
	if r.URL.Path == "/routes" {
		for _, di := range h.data {
			if di.DeletedAt == "" {
				responseData = append(responseData, di)
			}
		}
	} else {
		lm := path.Base(r.URL.Path)
		if lm == updatePathRoot {
			lm = ""
		}

		responseData = []*routeData{}
		for _, di := range h.data {
			if di.CreatedAt > lm || di.DeletedAt > lm {
				responseData = append(responseData, di)
			}
		}
	}

	if b, err := json.Marshal(responseData); err == nil {
		w.Write(b)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func sortDoc(doc string) string {
	exps := strings.Split(doc, ";")
	for i, exp := range exps {
		exps[i] = strings.Trim(exp, " \n")
	}
	sort.Strings(exps)
	return strings.Join(exps, ";")
}

func checkDoc(out string, in []*routeData) bool {
	c := &Client{routeCache: make(routeCache)}
	c.updateDoc(in)
	return sortDoc(toDocument(c.routeCache)) == sortDoc(out)
}

func testData() []*routeData {
	return []*routeData{
		&routeData{1, "", "", routeDef{
			"", nil, nil,
			pathMatch{pathMatchStrict, "/"},
			nil, nil, nil,
			endpoint{endpointReverseProxy, "https", "example.org", 443, "/"}}},
		&routeData{2, "", "2015-09-28T16:58:56.956", routeDef{
			"", nil, nil,
			pathMatch{pathMatchStrict, "/catalog"},
			nil, nil, nil,
			endpoint{endpointReverseProxy, "https", "example.org", 443, "/catalog"}}},
		&routeData{3, "", "", routeDef{
			"", nil, nil,
			pathMatch{pathMatchStrict, "/catalog"},
			nil, nil, nil,
			endpoint{endpointReverseProxy, "https", "example.org", 443, "/new-catalog"}}}}
}

func TestNothingToReceive(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	api := httptest.NewServer(http.NotFoundHandler())
	defer api.Close()

	c, err := Make(Options{api.URL, false, pollingTimeout, autoAuth(true), nil, nil})
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Close()

	select {
	case <-c.Receive():
		t.Error("shoudn't have received anything")
	case <-time.After(2 * pollingTimeout):
		// test done
	}
}

func TestReceiveInitialDataImmediately(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	data := testData()
	api := httptest.NewServer(&innkeeperHandler{data})
	defer api.Close()

	c, err := Make(Options{api.URL, false, pollingTimeout, autoAuth(true), nil, nil})
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Close()

	select {
	case doc := <-c.Receive():
		if !checkDoc(doc, []*routeData{
			&routeData{1, "", "", routeDef{
				"", nil, nil,
				pathMatch{pathMatchStrict, "/"},
				nil, nil, nil,
				endpoint{endpointReverseProxy, "https", "example.org", 443, "/"}}},
			&routeData{3, "", "", routeDef{
				"", nil, nil,
				pathMatch{pathMatchStrict, "/catalog"},
				nil, nil, nil,
				endpoint{endpointReverseProxy, "https", "example.org", 443, "/new-catalog"}}}}) {

			t.Error("failed to receive the right data")
		}
	case <-time.After(2 * pollingTimeout):
		t.Error("timeout")
	}
}

func TestReceiveNew(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	data := testData()
	h := &innkeeperHandler{data}
	api := httptest.NewServer(h)
	defer api.Close()

	c, err := Make(Options{api.URL, false, pollingTimeout, autoAuth(true), nil, nil})
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Close()

	// receive initial
	select {
	case <-c.Receive():
	case <-time.After(2 * pollingTimeout):
		t.Error("timeout")
	}

	// make a change
	h.data = append(data, &routeData{4, "2015-09-28T16:58:56.957", "", routeDef{
		"", nil, nil,
		pathMatch{pathMatchStrict, "/pdp"},
		nil, nil, nil,
		endpoint{endpointReverseProxy, "https", "example.org", 443, "/pdp"}}})

	// wait for the change
	select {
	case doc := <-c.Receive():
		if !checkDoc(doc, []*routeData{
			&routeData{1, "", "", routeDef{
				"", nil, nil,
				pathMatch{pathMatchStrict, "/"},
				nil, nil, nil,
				endpoint{endpointReverseProxy, "https", "example.org", 443, "/"}}},
			&routeData{3, "", "", routeDef{
				"", nil, nil,
				pathMatch{pathMatchStrict, "/catalog"},
				nil, nil, nil,
				endpoint{endpointReverseProxy, "https", "example.org", 443, "/new-catalog"}}},
			&routeData{4, "", "", routeDef{
				"", nil, nil,
				pathMatch{pathMatchStrict, "/pdp"},
				nil, nil, nil,
				endpoint{endpointReverseProxy, "https", "example.org", 443, "/pdp"}}}}) {
			t.Error("failed to receive the right data")
		}
	case <-time.After(2 * pollingTimeout):
		t.Error("timeout")
	}
}

func TestReceiveUpdate(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	data := testData()
	api := httptest.NewServer(&innkeeperHandler{data})
	defer api.Close()

	c, err := Make(Options{api.URL, false, pollingTimeout, autoAuth(true), nil, nil})
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Close()

	// receive initial
	select {
	case <-c.Receive():
	case <-time.After(2 * pollingTimeout):
		t.Error("timeout")
	}

	// make a change
	data[2] = &routeData{3, "2015-09-28T16:58:56.957", "", routeDef{
		"", nil, nil,
		pathMatch{pathMatchStrict, "/extra-catalog"},
		nil, nil, nil,
		endpoint{endpointReverseProxy, "https", "example.org", 443, "/extra-catalog"}}}

	// wait for the change
	select {
	case doc := <-c.Receive():
		if !checkDoc(doc, []*routeData{
			&routeData{1, "", "", routeDef{
				"", nil, nil,
				pathMatch{pathMatchStrict, "/"},
				nil, nil, nil,
				endpoint{endpointReverseProxy, "https", "example.org", 443, "/"}}},
			&routeData{3, "", "", routeDef{
				"", nil, nil,
				pathMatch{pathMatchStrict, "/extra-catalog"},
				nil, nil, nil,
				endpoint{endpointReverseProxy, "https", "example.org", 443, "/extra-catalog"}}}}) {
			t.Error("failed to receive the right data", doc)
		}
	case <-time.After(2 * pollingTimeout):
		t.Error("timeout")
	}
}

func TestReceiveDelete(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	data := testData()
	api := httptest.NewServer(&innkeeperHandler{data})
	defer api.Close()

	c, err := Make(Options{api.URL, false, pollingTimeout, autoAuth(true), nil, nil})
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Close()

	// receive initial
	select {
	case <-c.Receive():
	case <-time.After(2 * pollingTimeout):
		t.Error("timeout")
	}

	// make a change
	data[2].DeletedAt = "2015-09-28T16:58:56.957"

	// wait for the change
	select {
	case doc := <-c.Receive():
		if !checkDoc(doc, []*routeData{
			&routeData{1, "", "", routeDef{
				"", nil, nil,
				pathMatch{pathMatchStrict, "/"},
				nil, nil, nil,
				endpoint{endpointReverseProxy, "https", "example.org", 443, "/"}}}}) {
			t.Error("failed to receive the right data")
		}
	case <-time.After(2 * pollingTimeout):
		t.Error("timeout")
	}
}

func TestNoChange(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	data := testData()
	api := httptest.NewServer(&innkeeperHandler{data})
	defer api.Close()

	c, err := Make(Options{api.URL, false, pollingTimeout, autoAuth(true), nil, nil})
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Close()

	// receive initial
	select {
	case <-c.Receive():
	case <-time.After(2 * pollingTimeout):
		t.Error("timeout")
	}

	// check if receives anything
	select {
	case <-c.Receive():
		t.Error("shouldn't have received a change")
	case <-time.After(2 * pollingTimeout):
		// test done
	}
}

func TestAuthFailedInitial(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	data := testData()
	api := httptest.NewServer(&innkeeperHandler{data})
	defer api.Close()

	c, err := Make(Options{api.URL, false, pollingTimeout, autoAuth(false), nil, nil})
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Close()

	select {
	case <-c.Receive():
		t.Error("should not have received anything")
	case <-time.After(pollingTimeout):
		// test done
	}
}

func TestAuthFailedUpdate(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	data := testData()
	api := httptest.NewServer(&innkeeperHandler{data})
	defer api.Close()

	c, err := Make(Options{api.URL, false, pollingTimeout, autoAuth(true), nil, nil})
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Close()

	select {
	case <-c.Receive():
	case <-time.After(pollingTimeout):
		t.Error("timeout")
	}

	c.auth = autoAuth(false)
	select {
	case <-c.Receive():
		t.Error("should not have received anything")
	case <-time.After(pollingTimeout):
		// test done
	}
}

func TestAuthWithFixedToken(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	data := testData()
	api := httptest.NewServer(&innkeeperHandler{data})
	defer api.Close()

	c, err := Make(Options{api.URL, false, pollingTimeout, autoAuth(true), nil, nil})
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Close()

	select {
	case doc := <-c.Receive():
		if !checkDoc(doc, []*routeData{
			&routeData{1, "", "", routeDef{
				"", nil, nil,
				pathMatch{pathMatchStrict, "/"},
				nil, nil, nil,
				endpoint{endpointReverseProxy, "https", "example.org", 443, "/"}}},
			&routeData{3, "", "", routeDef{
				"", nil, nil,
				pathMatch{pathMatchStrict, "/catalog"},
				nil, nil, nil,
				endpoint{endpointReverseProxy, "https", "example.org", 443, "/new-catalog"}}}}) {

			t.Error("failed to receive the right data")
		}
	case <-time.After(pollingTimeout):
		t.Error("timeout")
	}
}

func TestParsingInnkeeperRoute(t *testing.T) {
	const testInnkeeperRoute = `{
        "id": 1,
        "createdAt": "2015-09-28T16:58:56.955",
        "deletedAt": "2015-09-28T16:58:56.956",
        "route": {
            "description": "The New Route",
            "match_methods": ["GET"],
            "match_headers": [
                {"name": "header0", "value": "value0"},
                {"name": "header1", "value": "value1"}
            ],
            "match_path": {
                "match": "/route",
                "type": "STRICT"
            },
            "rewrite_path": {
                "match": "_",
                "replace": "-"
            },
            "request_headers": [
                {"name": "header2", "value": "value2"},
                {"name": "header3", "value": "value3"}
            ],
            "response_headers": [
                {"name": "header4", "value": "value4"},
                {"name": "header5", "value": "value5"}
            ],
            "endpoint": {
                "hostname": "domain.eu",
                "port": 443,
                "protocol": "HTTPS",
                "type": "REVERSE_PROXY"
            }
        }
    }`

	r := routeData{}
	err := json.Unmarshal([]byte(testInnkeeperRoute), &r)
	if err != nil {
		t.Error(err)
	}

	if r.Id != 1 || r.CreatedAt != "2015-09-28T16:58:56.955" || r.DeletedAt != "2015-09-28T16:58:56.956" {
		t.Error("failed to parse route data")
	}

	if len(r.Route.MatchMethods) != 1 || r.Route.MatchMethods[0] != "GET" {
		t.Error("failed to parse methods")
	}

	if len(r.Route.MatchHeaders) != 2 ||
		r.Route.MatchHeaders[0].Name != "header0" || r.Route.MatchHeaders[0].Value != "value0" ||
		r.Route.MatchHeaders[1].Name != "header1" || r.Route.MatchHeaders[1].Value != "value1" {
		t.Error("failed to parse methods")
	}

	if r.Route.MatchPath.Typ != "STRICT" || r.Route.MatchPath.Match != "/route" {
		t.Error("failed to parse path match", r.Route.MatchPath.Typ, r.Route.MatchPath.Match)
	}

	if r.Route.RewritePath == nil || r.Route.RewritePath.Match != "_" || r.Route.RewritePath.Replace != "-" {
		t.Error("failed to path rewrite")
	}

	if len(r.Route.RequestHeaders) != 2 ||
		r.Route.RequestHeaders[0].Name != "header2" || r.Route.RequestHeaders[0].Name != "header2" ||
		r.Route.RequestHeaders[1].Name != "header3" || r.Route.RequestHeaders[1].Name != "header3" {
		t.Error("failed to parse request headers")
	}

	if len(r.Route.ResponseHeaders) != 2 ||
		r.Route.ResponseHeaders[0].Name != "header4" || r.Route.ResponseHeaders[0].Name != "header4" ||
		r.Route.ResponseHeaders[1].Name != "header5" || r.Route.ResponseHeaders[1].Name != "header5" {
		t.Error("failed to parse request headers")
	}

	if r.Route.Endpoint.Hostname != "domain.eu" ||
		r.Route.Endpoint.Port != 443 ||
		r.Route.Endpoint.Protocol != "HTTPS" ||
		r.Route.Endpoint.Typ != "REVERSE_PROXY" {
		t.Error("failed to parse endpoint")
	}
}

func TestParsingInnkeeperRouteNoPathRewrite(t *testing.T) {
	const testInnkeeperRoute = `{
        "id": 1,
        "route": {}
    }`

	r := routeData{}
	err := json.Unmarshal([]byte(testInnkeeperRoute), &r)
	if err != nil {
		t.Error(err)
	}

	if r.Route.RewritePath != nil {
		t.Error("failed to path rewrite")
	}
}

func TestParsingMultipleInnkeeperRoutes(t *testing.T) {
	const testInnkeeperRoutes = `[{
        "id": 1,
        "route": {
            "description": "The New Route",
            "match_path": {
                "match": "/route",
                "type": "STRICT"
            },
            "endpoint": {
                "hostname": "domain.eu"
            }
        }
    }, {
        "id": 2,
        "route": {
            "description": "The New Route",
            "match_path": {
                "match": "/route",
                "type": "STRICT"
            },
            "endpoint": {
                "hostname": "domain.eu"
            }
        }
    }]`

	rs := []*routeData{}
	err := json.Unmarshal([]byte(testInnkeeperRoutes), &rs)
	if err != nil {
		t.Error(err)
	}

	if len(rs) != 2 || rs[0].Id != 1 || rs[1].Id != 2 {
		t.Error("failed to parse routes")
	}
}

func TestParsingMultipleInnkeeperRoutesWithDelete(t *testing.T) {
	const testInnkeeperRoutes = `[{"id": 1}, {"id": 2, "deletedAt": "2015-09-28T16:58:56.956"}]`

	rs := []*routeData{}
	err := json.Unmarshal([]byte(testInnkeeperRoutes), &rs)
	if err != nil {
		t.Error(err)
	}

	if len(rs) != 2 || rs[0].Id != 1 || rs[1].Id != 2 || rs[0].DeletedAt != "" || rs[1].DeletedAt != "2015-09-28T16:58:56.956" {
		t.Error("failed to parse routes")
	}
}

func TestProducesParsableDocument(t *testing.T) {
	const pollingTimeout = 15 * time.Millisecond
	data := testData()
	api := httptest.NewServer(&innkeeperHandler{data})
	defer api.Close()

	c, err := Make(Options{api.URL, false, pollingTimeout, autoAuth(true), nil, nil})
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Close()

	select {
	case doc := <-c.Receive():
		parsed, err := eskip.Parse(doc)
		if err != nil {
			t.Error(err)
		}

		if len(parsed) != 2 {
			t.Error("failed to parse")
		}
	case <-time.After(2 * pollingTimeout):
		t.Error("timeout")
	}
}
