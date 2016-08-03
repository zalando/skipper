package eskip

import (
	"testing"
)

func findDiffPos(left, right string) int {
	pos := 0
	for i := 0; i < len(left); i++ {
		if left[i:i+1] != right[i:i+1] {
			pos = i
			break
		}
	}

	return pos
}

func testDoc(t *testing.T, doc string) string {
	routes, err := Parse(doc)
	if err != nil {
		t.Error(err)
	}

	docBack := String(routes...)
	if docBack != doc {
		t.Error("failed to serialize doc", findDiffPos(docBack, doc))
		t.Log(docBack)
		t.Log(doc)
	}

	return docBack
}

func TestRouteString(t *testing.T) {
	for i, item := range []struct {
		route  *Route
		string string
	}{{
		&Route{},
		`* -> ""`,
	}, {
		&Route{Method: "GET", Backend: "https://www.example.org"},
		`Method("GET") -> "https://www.example.org"`,
	}, {
		&Route{
			Path:        `/some/"/path`,
			HostRegexps: []string{"h-expression", "slash/h-expression"},
			PathRegexps: []string{"p-expression", "slash/p-expression"},
			Method:      "PUT",
			Headers: map[string]string{
				`ap"key`: `ap"value`},
			HeaderRegexps: map[string][]string{
				`ap"key`: []string{"slash/value0", "slash/value1"}},
			Predicates: []*Predicate{{"Test", []interface{}{3.14, "hello"}}},
			Filters: []*Filter{
				{"filter0", []interface{}{float64(3.1415), "argvalue"}},
				{"filter1", []interface{}{float64(-42), `ap"argvalue`}}},
			Shunt:   false,
			Backend: "https://www.example.org"},
		`Path("/some/\"/path") && Host(/h-expression/) && ` +
			`Host(/slash\/h-expression/) && ` +
			`PathRegexp(/p-expression/) && PathRegexp(/slash\/p-expression/) && ` +
			`Method("PUT") && ` +
			`Header("ap\"key", "ap\"value") && ` +
			`HeaderRegexp("ap\"key", /slash\/value0/) && HeaderRegexp("ap\"key", /slash\/value1/) && ` +
			`Test(3.14, "hello") -> ` +
			`filter0(3.1415, "argvalue") -> filter1(-42, "ap\"argvalue") -> ` +
			`"https://www.example.org"`,
	}, {
		&Route{
			Method:  "GET",
			Filters: []*Filter{{"static", []interface{}{"/some", "/file"}}},
			Shunt:   true},
		`Method("GET") -> static("/some", "/file") -> <shunt>`,
	}} {
		rstring := item.route.String()
		if rstring != item.string {
			pos := findDiffPos(rstring, item.string)
			t.Error(rstring, item.string, i, pos, rstring[pos:pos+1], item.string[pos:pos+1])
		}
	}
}

func TestRouteExpression(t *testing.T) {
	r := &Route{Method: "GET", Backend: "https://www.example.org"}
	if r.String() != `Method("GET") -> "https://www.example.org"` {
		t.Error("failed to serialize a route expression")
	}
}

func TestDocString(t *testing.T) {
	testDoc(t, `route1: Method("GET") -> filter("expression") -> <shunt>;`+"\n"+
		`route2: Path("/some/path") -> "https://www.example.org"`)
}

func TestPrintNonPretty(t *testing.T) {
	for i, item := range []struct {
		route    string
		expected string
	}{
		{
			"route1: Method(\"GET\") -> filter(\"expression\") -> <shunt>",
			"Method(\"GET\") -> filter(\"expression\") -> <shunt>",
		},
		{
			"route2: Path(\"/some/path\") -> \"https://www.example.org\"",
			"Path(\"/some/path\") -> \"https://www.example.org\"",
		},
	} {
		testPrinting(item.route, item.expected, t, i, false)
	}
}

func TestPrintPretty(t *testing.T) {
	for i, item := range []struct {
		route    string
		expected string
	}{
		{
			"route1: Method(\"GET\") -> filter(\"expression\") -> <shunt>",
			"Method(\"GET\")\n  -> filter(\"expression\")\n  -> <shunt>",
		},
		{
			"route2: Path(\"/some/path\") -> \"https://www.example.org\"",
			"Path(\"/some/path\")\n  -> \"https://www.example.org\"",
		},
	} {
		testPrinting(item.route, item.expected, t, i, true)
	}
}

func testPrinting(routestr string, expected string, t *testing.T, i int, pretty bool) {
	route, err := Parse(routestr)
	if err != nil {
		t.Error(err)
	}

	printedRoute := route[0].Print(pretty)

	if printedRoute != expected {
		pos := findDiffPos(printedRoute, expected)
		t.Error(printedRoute, expected, i, pos, printedRoute[pos:pos+1], expected[pos:pos+1])
	}
}

func TestParseAndStringAndParse(t *testing.T) {
	doc := `route1: Method("GET") -> filter("expression") -> <shunt>;` + "\n" +
		`route2: Path("/some/path") -> "https://www.example.org"`
	doc = testDoc(t, doc)
	doc = testDoc(t, doc)
	doc = testDoc(t, doc)
}
