package innkeeper

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

	"github.com/zalando/skipper/eskip"
)

const testAuthenticationToken = "test token"

const updatePathRoot = "updated-routes"

type autoAuth bool

func (aa autoAuth) GetToken() (string, error) {
	if aa {
		return testAuthenticationToken, nil
	}

	return "", errors.New(string(authErrorMissingCredentials))
}

type innkeeperHandler struct{ data []*routeData }

func (h *innkeeperHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(authHeaderName) != "Bearer "+testAuthenticationToken {
		w.WriteHeader(http.StatusUnauthorized)
		enc := json.NewEncoder(w)

		// ignoring error
		enc.Encode(&apiError{ErrorType: string(authErrorAuthorization)})

		return
	}

	var responseData []*routeData
	if r.URL.Path == "/current-routes" {
		for _, di := range h.data {
			if !di.Action.Delete() {
				responseData = append(responseData, di)
			}
		}
	} else {
		lastMod := path.Base(r.URL.Path)
		if lastMod == updatePathRoot {
			lastMod = ""
		}

		for _, di := range h.data {
			if di.Timestamp > lastMod {
				responseData = append(responseData, di)
			}
		}
	}

	if b, err := json.Marshal(responseData); err == nil {
		w.Write(b)
	} else {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func innkeeperServer(data []*routeData) *httptest.Server {
	return httptest.NewServer(&innkeeperHandler{data})
}

func checkDoc(t *testing.T, rs []*eskip.Route, ds []string, d []*routeData) bool {
	check, eds, _ := convertJsonToEskip(d, nil, nil)

	if len(rs) != len(check) {
		t.Error("doc lengths do not match", len(rs), len(check))
		return false
	}

	for i, r := range rs {
		if r.Id != check[i].Id {
			t.Error("doc id does not match")
			return false
		}

		if r.Path != check[i].Path {
			t.Error("doc path does not match")
			return false
		}

		if len(r.PathRegexps) != len(check[i].PathRegexps) {
			t.Error("doc path regexp lengths do not match")
			return false
		}

		for j, rx := range r.PathRegexps {
			if rx != check[i].PathRegexps[j] {
				t.Error("doc path regexp does not match")
				return false
			}
		}

		if r.Method != check[i].Method {
			t.Error("doc method does not match")
			return false
		}

		if len(r.Headers) != len(check[i].Headers) {
			t.Error("doc header lengths do not match")
			return false
		}

		for k, h := range r.Headers {
			if h != check[i].Headers[k] {
				t.Error("doc header does not match")
				return false
			}
		}

		if len(r.Filters) != len(check[i].Filters) {
			t.Error("doc filter lengths do not match")
			return false
		}

		for j, f := range r.Filters {
			if f.Name != check[i].Filters[j].Name {
				t.Error("doc filter does not match")
				return false
			}

			if len(f.Args) != len(check[i].Filters[j].Args) {
				t.Error("doc filter arg lengths do not match")
				return false
			}

			for k, a := range f.Args {
				if a != check[i].Filters[j].Args[k] {
					t.Error("doc filter arg does not match")
					return false
				}
			}
		}

		if r.Shunt != check[i].Shunt {
			t.Error("doc shunt does not match")
			return false
		}

		if r.Backend != check[i].Backend {
			t.Error("doc backend does not match")
			return false
		}
	}

	if len(ds) != len(eds) {
		t.Error("number of deleted ids doesn't match")
		return false
	}

	for _, edsi := range eds {
		found := false
		for _, dsi := range ds {
			if dsi == edsi {
				found = true
			}
		}

		if !found {
			t.Error("deleted ids don't match")
			return false
		}
	}

	return true
}

func TestParsingInnkeeperSimpleRoute(t *testing.T) {
	const testInnkeeperRoute = `{
		"name": "THE_ROUTE",
		"timestamp": "2015-09-28T16:58:56.957",
		"type": "delete",
		"eskip": "THE_ROUTE: PathRegexp(\"/hello-.*\") -> <shunt>"
	}`

	r := routeData{}
	err := json.Unmarshal([]byte(testInnkeeperRoute), &r)
	if err != nil {
		t.Error(err)
	}

	if r.Name != "THE_ROUTE" {
		t.Error("failed to parse the name")
	}

	if r.Timestamp != "2015-09-28T16:58:56.957" || !r.Action.Delete() {
		t.Error("failed to parse route data")
	}

	if r.Eskip != "THE_ROUTE: PathRegexp(\"/hello-.*\") -> <shunt>" {
		t.Error("route eskip")
	}
}

func TestParsingInnkeeperComplexRoute(t *testing.T) {
	const testInnkeeperRoute = `{
		"name": "THE_ROUTE",
		"timestamp": "2015-09-28T16:58:56.956",
		"type": "delete",
		"eskip": "PathRegexp(\"/hello-.*\") -> someFilter(\"Hello\", 123) -> \"https://www.example.org\""
	}`

	r := routeData{}
	err := json.Unmarshal([]byte(testInnkeeperRoute), &r)
	if err != nil {
		t.Error(err)
	}

	if r.Name != "THE_ROUTE" {
		t.Error("failed to parse the name")
	}

	if r.Timestamp != "2015-09-28T16:58:56.956" || !r.Action.Delete() {
		t.Error("failed to parse route data")
	}

	if r.Eskip != "PathRegexp(\"/hello-.*\") -> someFilter(\"Hello\", 123) -> \"https://www.example.org\"" {
		t.Error("failed to parse route eskip")
	}
}

func TestParsingMultipleInnkeeperRoutes(t *testing.T) {
	const testInnkeeperRoutes = `[{
		"name": "THE_ROUTE",
		"timestamp": "2015-09-28T16:58:56.957",
		"type": "create",
		"eskip": "PathRegexp(\"/hello-.*\") -> \"https://www.example.org\""
	}, {
		"name": "ANOTHER_ROUTE",
		"timestamp": "2015-09-28T16:58:56.957",
		"type": "delete",
		"eskip": "PathRegexp(\"/hello-.*\") -> \"https://www.example.org\""
	}]`

	rs := []*routeData{}
	err := json.Unmarshal([]byte(testInnkeeperRoutes), &rs)
	if err != nil {
		t.Error(err)
	}

	if len(rs) != 2 || rs[0].Name != "THE_ROUTE" || rs[1].Name != "ANOTHER_ROUTE" {
		t.Error("failed to parse routes")
	}
}

func TestConvertDoc(t *testing.T) {
	testData := []*routeData{
		{
			Name:      "route1",
			Action:    createAction,
			Timestamp: "2015-09-28T16:58:56.955",
			Eskip:     `route1: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, {
			Name:      "route2",
			Action:    deleteAction,
			Timestamp: "2015-09-28T16:58:56.956",
			Eskip:     `route2: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, {
			Name:      "route3",
			Action:    deleteAction,
			Timestamp: "2015-09-28T16:58:56.956",
			Eskip:     `route3: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, {
			Name:      "route4",
			Action:    updateAction,
			Timestamp: "2015-09-28T16:58:56.957",
			Eskip: `route4: Path("/catalog") && Method("GET")
				-> modPath(".*", "/new-catalog")
				-> "https://catalog.example.org:443"`}}

	failed := false

	test := func(left, right interface{}, msg ...interface{}) {
		if failed || left == right {
			return
		}

		failed = true
		t.Error(append([]interface{}{"failed to convert data", left, right}, msg...)...)
	}

	rs, deleted, lastChange := convertJsonToEskip(testData, nil, nil)

	test(len(rs), 2)
	if failed {
		return
	}

	test(rs[0].Id, "route1")
	test(rs[0].Path, "/")
	test(rs[0].Shunt, false)
	test(rs[0].Backend, "https://example.org:443")

	test(rs[1].Id, "route4")
	test(rs[1].Path, "/catalog")
	test(rs[1].Shunt, false)
	test(rs[1].Backend, "https://catalog.example.org:443")

	test(len(deleted), 2)
	test(lastChange, "2015-09-28T16:58:56.957")
}

func TestReceivesEmpty(t *testing.T) {
	s := innkeeperServer(nil)
	defer s.Close()

	c, err := New(Options{Address: s.URL, Authentication: autoAuth(true)})
	if err != nil {
		t.Error(err)
		return
	}

	rs, err := c.LoadAll()
	if err != nil || len(rs) != 0 {
		t.Error(err, "failed to receive empty")
	}
}

func TestReceive(t *testing.T) {
	h := &innkeeperHandler{}
	s := httptest.NewServer(h)
	defer s.Close()

	for _, ti := range []struct {
		msg                      string
		data, update             []*routeData
		auth, updateAuth         bool
		expected, expectedUpdate []*routeData
		err, updateErr           bool
	}{{
		msg:  "receives empty",
		auth: true,
	}, {
		msg: "receives expected",
		data: []*routeData{{
			Name:      "route1",
			Action:    createAction,
			Timestamp: "2015-09-28T16:58:56.955",
			Eskip:     `route1: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, {
			Name:      "route4",
			Action:    updateAction,
			Timestamp: "2015-09-28T16:58:56.957",
			Eskip: `route4: Path("/catalog") && Method("GET")
				-> modPath(".*", "/new-catalog")
				-> "https://catalog.example.org:443"`}},
		auth: true,
		expected: []*routeData{{
			Name:  "route1",
			Eskip: `route1: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, {
			Name: "route4",
			Eskip: `route4: Path("/catalog") && Method("GET")
				-> modPath(".*", "/new-catalog")
				-> "https://catalog.example.org:443"`}},
	}, {
		msg: "fails on auth when receiving",
		data: []*routeData{{
			Name:      "route1",
			Action:    createAction,
			Timestamp: "2015-09-28T16:58:56.955",
			Eskip:     `route1: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, {
			Name:      "route4",
			Action:    updateAction,
			Timestamp: "2015-09-28T16:58:56.957",
			Eskip: `route4: Path("/catalog") && Method("GET")
				-> modPath(".*", "/new-catalog")
				-> "https://catalog.example.org:443"`}},
		auth: false,
		err:  true,
	}, {
		msg: "receives updates",
		data: []*routeData{{
			Name:      "route1",
			Action:    createAction,
			Timestamp: "2015-09-28T16:58:56.955",
			Eskip:     `route1: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, {
			Name:      "route4",
			Action:    updateAction,
			Timestamp: "2015-09-28T16:58:56.957",
			Eskip: `route4: Path("/catalog") && Method("GET")
				-> modPath(".*", "/new-catalog")
				-> "https://catalog.example.org:443"`}},
		update: []*routeData{{
			Name:      "route1",
			Action:    updateAction,
			Timestamp: "2015-09-28T16:58:56.958",
			Eskip:     `route1: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, {
			Name:      "route3",
			Action:    createAction,
			Timestamp: "2015-09-28T16:58:56.959",
			Eskip:     `route1: Path("/put-something") && Method("PUT") -> "https://example.org:443"`,
		}, {
			Name:      "route4",
			Action:    deleteAction,
			Timestamp: "2015-09-28T16:58:56.960",
			Eskip: `route4: Path("/catalog") && Method("GET")
				-> modPath(".*", "/new-catalog")
				-> "https://catalog.example.org:443"`}},
		auth:       true,
		updateAuth: true,
		expected: []*routeData{{
			Name:  "route1",
			Eskip: `route1: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, {
			Name: "route4",
			Eskip: `route4: Path("/catalog") && Method("GET")
				-> modPath(".*", "/new-catalog")
				-> "https://catalog.example.org:443"`}},
		expectedUpdate: []*routeData{{
			Name:  "route1",
			Eskip: `route1: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, {
			Name:  "route3",
			Eskip: `route1: Path("/put-something") && Method("PUT") -> "https://example.org:443"`,
		}, {
			Name:   "route4",
			Action: deleteAction,
			Eskip: `route4: Path("/catalog") && Method("GET")
				-> modPath(".*", "/new-catalog")
				-> "https://catalog.example.org:443"`}},
	}, {
		msg: "failing auth on update",
		data: []*routeData{{
			Name:      "route1",
			Action:    createAction,
			Timestamp: "2015-09-28T16:58:56.955",
			Eskip:     `route1: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, {
			Name:      "route4",
			Action:    updateAction,
			Timestamp: "2015-09-28T16:58:56.957",
			Eskip: `route4: Path("/catalog") && Method("GET")
				-> modPath(".*", "/new-catalog")
				-> "https://catalog.example.org:443"`}},
		update: []*routeData{{
			Name:      "route1",
			Action:    updateAction,
			Timestamp: "2015-09-28T16:58:56.958",
			Eskip:     `route1: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, {
			Name:      "route3",
			Action:    createAction,
			Timestamp: "2015-09-28T16:58:56.959",
			Eskip:     `route1: Path("/put-something") && Method("PUT") -> "https://example.org:443"`,
		}, {
			Name:      "route4",
			Action:    deleteAction,
			Timestamp: "2015-09-28T16:58:56.960",
			Eskip: `route4: Path("/catalog") && Method("GET")
				-> modPath(".*", "/new-catalog")
				-> "https://catalog.example.org:443"`}},
		auth:       true,
		updateAuth: false,
		expected: []*routeData{{
			Name:  "route1",
			Eskip: `route1: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, {
			Name: "route4",
			Eskip: `route4: Path("/catalog") && Method("GET")
				-> modPath(".*", "/new-catalog")
				-> "https://catalog.example.org:443"`}},
		updateErr: true,
	}} {
		h.data = ti.data

		c, err := New(Options{Address: s.URL, Authentication: autoAuth(ti.auth)})
		if err != nil {
			t.Error(ti.msg, err)
			continue
		}

		rs, err := c.LoadAll()
		if err == nil && ti.err {
			t.Error(ti.msg, "failed to fail")
			continue
		} else if err != nil && !ti.err {
			t.Error(ti.msg, err)
			continue
		}

		if ti.err {
			continue
		}

		if !checkDoc(t, rs, nil, ti.expected) {
			continue
		}

		if len(ti.update) == 0 {
			continue
		}

		h.data = ti.update
		c.authToken = ""
		c.opts.Authentication = autoAuth(ti.updateAuth)
		rs, ds, err := c.LoadUpdate()
		if err == nil && ti.updateErr {
			t.Error(ti.msg, "failed to fail on update")
			continue
		} else if err != nil && !ti.updateErr {
			t.Error(ti.msg, err)
			continue
		}

		checkDoc(t, rs, ds, ti.expectedUpdate)
	}
}

func TestUsesPreAndPostRouteFilters(t *testing.T) {
	d := []*routeData{{
		Name:      "route1",
		Action:    createAction,
		Timestamp: "2015-09-28T16:58:56.955",
		Eskip: `route1: Path("/") && Method("GET")
			-> modPath(".*", "/replacement")
			-> "https://example.org:443"`,
	}, {
		Name:      "route4",
		Action:    updateAction,
		Timestamp: "2015-09-28T16:58:56.957",
		Eskip: `route4: Path("/catalog") && Method("GET")
			-> modPath(".*", "/replacement")
			-> "https://catalog.example.org:443"`}}

	s := innkeeperServer(d)
	defer s.Close()

	c, err := New(Options{
		Address:          s.URL,
		Authentication:   autoAuth(true),
		PreRouteFilters:  `filter1(3.14) -> filter2("key", 42)`,
		PostRouteFilters: `filter3("Hello, world!")`})
	if err != nil {
		t.Error(err)
		return
	}

	rs, err := c.LoadAll()
	if err != nil {
		t.Error(err)
	}

	for _, r := range rs {
		if len(r.Filters) != 4 {
			t.Error("failed to parse filters 1")
		}

		if r.Filters[0].Name != "filter1" ||
			len(r.Filters[0].Args) != 1 ||
			r.Filters[0].Args[0] != float64(3.14) {
			t.Error("failed to parse filters 2")
		}

		if r.Filters[1].Name != "filter2" ||
			len(r.Filters[1].Args) != 2 ||
			r.Filters[1].Args[0] != "key" ||
			r.Filters[1].Args[1] != float64(42) {
			t.Error("failed to parse filters 3")
		}

		if r.Filters[2].Name != "modPath" ||
			len(r.Filters[2].Args) != 2 ||
			r.Filters[2].Args[0] != ".*" ||
			r.Filters[2].Args[1] != "/replacement" {
			t.Error("failed to parse filters 4")
		}

		if r.Filters[3].Name != "filter3" ||
			len(r.Filters[3].Args) != 1 ||
			r.Filters[3].Args[0] != "Hello, world!" {
			t.Error("failed to parse filters 5")
		}
	}
}
