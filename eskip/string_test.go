package eskip

import (
	"testing"
)

func findDiffPos(left, right string) int {
	if len(left) != len(right) {
		return -1
	}

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
		`Any() -> ""`,
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
			`HeaderRegexp("ap\"key", /slash\/value0/) && HeaderRegexp("ap\"key", /slash\/value1/) -> ` +
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

func TestDocString(t *testing.T) {
	testDoc(t, `route1: Method("GET") -> filter("expression") -> <shunt>;`+"\n"+
		`route2: Path("/some/path") -> "https://www.example.org"`)
}

func TestParseAndStringAndParse(t *testing.T) {
	doc := `route1: Method("GET") -> filter("expression") -> <shunt>;` + "\n" +
		`route2: Path("/some/path") -> "https://www.example.org"`
	doc = testDoc(t, doc)
	doc = testDoc(t, doc)
	doc = testDoc(t, doc)
}
