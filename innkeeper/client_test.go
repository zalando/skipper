package innkeeper

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
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
			if di.Action == createAction || di.Action == updateAction {
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

func testData() []*routeData {
	return []*routeData{
		&routeData{
			Name:      "route1",
			Action:    createAction,
			Timestamp: "2015-09-28T16:58:56.955",
			Eskip:     `route1: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, &routeData{
			Name:      "route2",
			Action:    deleteAction,
			Timestamp: "2015-09-28T16:58:56.956",
			Eskip:     `route2: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, &routeData{
			Name:      "route3",
			Action:    deleteAction,
			Timestamp: "2015-09-28T16:58:56.956",
			Eskip:     `route3: Path("/") && Method("GET") -> "https://example.org:443"`,
		}, &routeData{
			Name:      "route4",
			Action:    updateAction,
			Timestamp: "2015-09-28T16:58:56.957",
			Eskip: `route4: Path("/catalog") && Method("GET")
				-> modPath(".*", "/new-catalog")
				-> "https://catalog.example.org:443"`,
		}}
}

func checkDoc(t *testing.T, rs []*eskip.Route, d []*routeData) {
	check, _, _ := convertJsonToEskip(d, nil, nil)
	if len(rs) != len(check) {
		t.Error("doc lengths do not match", len(rs), len(check))
		return
	}

	for i, r := range rs {
		if r.Id != check[i].Id {
			t.Error("doc id does not match")
			return
		}

		if r.Path != check[i].Path {
			t.Error("doc path does not match")
			return
		}

		if len(r.PathRegexps) != len(check[i].PathRegexps) {
			t.Error("doc path regexp lengths do not match")
			return
		}

		for j, rx := range r.PathRegexps {
			if rx != check[i].PathRegexps[j] {
				t.Error("doc path regexp does not match")
				return
			}
		}

		if r.Method != check[i].Method {
			t.Error("doc method does not match")
			return
		}

		if len(r.Headers) != len(check[i].Headers) {
			t.Error("doc header lengths do not match")
			return
		}

		for k, h := range r.Headers {
			if h != check[i].Headers[k] {
				t.Error("doc header does not match")
				return
			}
		}

		if len(r.Filters) != len(check[i].Filters) {
			t.Error("doc filter lengths do not match")
			return
		}

		for j, f := range r.Filters {
			if f.Name != check[i].Filters[j].Name {
				t.Error("doc filter does not match")
				return
			}

			if len(f.Args) != len(check[i].Filters[j].Args) {
				t.Error("doc filter arg lengths do not match")
				return
			}

			for k, a := range f.Args {
				if a != check[i].Filters[j].Args[k] {
					t.Error("doc filter arg does not match")
					return
				}
			}
		}

		if r.Shunt != check[i].Shunt {
			t.Error("doc shunt does not match")
			return
		}

		if r.Backend != check[i].Backend {
			t.Error("doc backend does not match")
			return
		}
	}
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

	if r.Timestamp != "2015-09-28T16:58:56.957" || r.Action != deleteAction {
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

	if r.Timestamp != "2015-09-28T16:58:56.956" || r.Action != deleteAction {
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
	failed := false

	test := func(left, right interface{}, msg ...interface{}) {
		if failed || left == right {
			return
		}

		failed = true
		t.Error(append([]interface{}{"failed to convert data", left, right}, msg...)...)
	}

	rs, deleted, lastChange := convertJsonToEskip(testData(), nil, nil)

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

func TestReceivesInitial(t *testing.T) {
	d := testData()
	s := innkeeperServer(d)
	defer s.Close()

	c, err := New(Options{Address: s.URL, Authentication: autoAuth(true)})
	if err != nil {
		t.Error(err)
		return
	}

	rs, err := c.LoadAll()
	if err != nil {
		t.Error(err)
	}

	checkDoc(t, rs, d)
}

func TestFailingAuthOnReceive(t *testing.T) {
	d := testData()
	s := innkeeperServer(d)
	defer s.Close()
	a := autoAuth(false)

	c, err := New(Options{Address: s.URL, Authentication: a})
	if err != nil {
		t.Error(err)
		return
	}

	_, err = c.LoadAll()
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestReceivesUpdates(t *testing.T) {
	d := testData()
	h := &innkeeperHandler{d}
	s := httptest.NewServer(h)

	c, err := New(Options{Address: s.URL, Authentication: autoAuth(true)})
	if err != nil {
		t.Error(err)
		return
	}

	c.LoadAll()

	d = testData()
	d[2].Timestamp = "2015-09-28T16:58:56.958"
	d[2].Action = deleteAction

	newRoute := &routeData{
		Name:      "route4",
		Timestamp: "2015-09-28T16:58:56.959",
		Action:    createAction,
		Eskip:     `Path("/") && Method("GET") -> "https://example.org"`,
	}

	d = append(d, newRoute)
	h.data = d

	rs, ds, err := c.LoadUpdate()
	if err != nil {
		t.Error(err)
	}

	checkDoc(t, rs, []*routeData{newRoute})
	if len(ds) != 1 || ds[0] != "route3" {
		t.Error("unexpected delete")
	}
}

func TestFailingAuthOnUpdate(t *testing.T) {
	d := testData()
	h := &innkeeperHandler{d}
	s := httptest.NewServer(h)

	c, err := New(Options{Address: s.URL, Authentication: autoAuth(true)})
	if err != nil {
		t.Error(err)
		return
	}

	c.LoadAll()

	c.authToken = ""
	c.opts.Authentication = autoAuth(false)
	d = testData()
	d[2].Timestamp = "2015-09-28T16:58:56.958"
	d[2].Action = deleteAction

	newRoute := &routeData{
		Name:      "route4",
		Timestamp: "2015-09-28T16:58:56.959",
		Action:    createAction,
		Eskip:     `Path("/") && Method("GET") -> "https://example.org"`,
	}

	d = append(d, newRoute)
	h.data = d

	_, _, err = c.LoadUpdate()
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestUsesPreAndPostRouteFilters(t *testing.T) {
	d := testData()[:3]
	for _, di := range d {
		di.Eskip = strings.Replace(di.Eskip, "->", "-> "+builtin.ModPathName+"(\".*\", \"replacement\") ->", 1)
	}

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

		if r.Filters[2].Name != builtin.ModPathName ||
			len(r.Filters[2].Args) != 2 ||
			r.Filters[2].Args[0] != ".*" ||
			r.Filters[2].Args[1] != "replacement" {
			t.Error("failed to parse filters 4")
		}

		if r.Filters[3].Name != "filter3" ||
			len(r.Filters[3].Args) != 1 ||
			r.Filters[3].Args[0] != "Hello, world!" {
			t.Error("failed to parse filters 5")
		}
	}
}
