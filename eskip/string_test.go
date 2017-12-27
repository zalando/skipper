package eskip

import (
	"bytes"
	"fmt"
	"testing"
)

func findDiffPos(left, right string) (pos int, leftOut, rightOut string) {
	for i := 0; i < len(left); i++ {
		if len(right) <= i {
			pos = i
			break
		}

		if left[i:i+1] != right[i:i+1] {
			pos = i
			break
		}
	}

	leftOut = left[0:pos]
	rightOut = right[0:pos]

	return
}

func testDoc(t *testing.T, doc string) string {
	routes, err := Parse(doc)
	if err != nil {
		t.Error(err)
	}

	docBack := String(routes...)
	if docBack != doc {
		pos, _, _ := findDiffPos(docBack, doc)
		t.Error("failed to serialize doc", pos)
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
				`ap"key`: {"slash/value0", "slash/value1"}},
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
	}, {
		&Route{
			Method:      "GET",
			Filters:     []*Filter{{"static", []interface{}{"/some", "/file"}}},
			BackendType: ShuntBackend},
		`Method("GET") -> static("/some", "/file") -> <shunt>`,
	}, {
		&Route{
			Method:      "GET",
			Filters:     []*Filter{{"static", []interface{}{"/some", "/file"}}},
			BackendType: LoopBackend},
		`Method("GET") -> static("/some", "/file") -> <loopback>`,
	}} {
		rstring := item.route.String()
		if rstring != item.string {
			pos, rstringOut, itemOut := findDiffPos(rstring, item.string)
			t.Error(rstring, item.string, i, pos, rstringOut, itemOut)
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
		`route2: Path("/some/path") -> "https://www.example.org";`)
}

func TestPrintNonPretty(t *testing.T) {
	for i, item := range []struct {
		route    string
		expected string
	}{
		{
			`route1: Method("GET") -> filter("expression") -> <shunt>`,
			`Method("GET") -> filter("expression") -> <shunt>`,
		},
		{
			`route2: Path("/some/path") -> "https://www.example.org"`,
			`Path("/some/path") -> "https://www.example.org"`,
		},
	} {
		testPrinting(item.route, item.expected, t, i, false, false)
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
		testPrinting(item.route, item.expected, t, i, true, false)
	}
}

func TestPrintMultiRoutePretty(t *testing.T) {
	testPrinting(`route1: Method("GET") -> filter("expression") -> <shunt>;`+"\n"+
		`route2: Path("/some/path") -> "https://www.example.org"`,
		`route1: Method("GET")`+"\n"+
			`  -> filter("expression")`+"\n"+
			`  -> <shunt>;`+"\n\n"+
			`route2: Path("/some/path")`+"\n"+
			`  -> "https://www.example.org";`,
		t, 0, true, true)
}

func TestPrintMultiRouteNonPretty(t *testing.T) {
	testPrinting(`route1: Method("GET") -> filter("expression") -> <shunt>;`+"\n"+
		`route2: Path("/some/path") -> "https://www.example.org"`,
		`route1: Method("GET") -> filter("expression") -> <shunt>;`+"\n"+
			`route2: Path("/some/path") -> "https://www.example.org";`,
		t, 0, false, true)
}

func testPrinting(routestr string, expected string, t *testing.T, i int, pretty bool, multi bool) {
	routes, err := Parse(routestr)
	if err != nil {
		t.Error(err)
	}
	var printedRoute string

	if multi {
		printedRoute = Print(pretty, routes...)
	} else {
		printedRoute = routes[0].Print(pretty)
	}

	if printedRoute != expected {
		pos, printed, expected := findDiffPos(printedRoute, expected)
		t.Error(printedRoute, expected, i, pos, printed, expected)
	}
}

func TestParseAndStringAndParse(t *testing.T) {
	doc := `route1: Method("GET") -> filter("expression") -> <shunt>;` + "\n" +
		`route2: Path("/some/path") -> "https://www.example.org";`
	doc = testDoc(t, doc)
	doc = testDoc(t, doc)
	doc = testDoc(t, doc)
}

func TestNumberString(t *testing.T) {
	for _, ti := range []float64{
		0,
		1,
		0.1,
		0.123,
		0.123456789,
		0.12345678901234568901234567890,
		123,
		123456789,
		123456789012345678901234567890,
	} {
		t.Run(fmt.Sprint(ti), func(t *testing.T) {
			in := &Route{Filters: []*Filter{{Name: "filter", Args: []interface{}{ti}}}}
			str := String(in)
			t.Log("output", str)
			out, err := Parse(str)
			if err != nil {
				t.Error(err)
				return
			}

			if len(out) != 1 || len(out[0].Filters) != 1 || len(out[0].Filters[0].Args) != 1 {
				t.Error("parse failed")
				return
			}

			if v, ok := out[0].Filters[0].Args[0].(float64); !ok || v != ti {
				t.Error("print/parse failed", v, ti)
			}
		})
	}
}

func TestPrintLines(t *testing.T) {
	check := func(t *testing.T, got, expected string) {
		if got != expected {
			t.Error("invalid string result")
			t.Log("got:     ", got)
			t.Log("expected:", expected)
		}
	}

	t.Run("route method", func(t *testing.T) {
		route := &Route{
			Predicates: []*Predicate{{
				Name: "Path",
				Args: []interface{}{
					"/foo",
				},
			}},
			Filters: []*Filter{{
				Name: "setPath",
				Args: []interface{}{
					"/",
				},
			}},
			Backend: "https://www.example.org",
		}

		t.Run("String()", func(t *testing.T) {
			expected := `Path("/foo") -> setPath("/") -> "https://www.example.org"`
			got := route.String()
			check(t, got, expected)
		})

		t.Run("Print()", func(t *testing.T) {
			t.Run("not pretty", func(t *testing.T) {
				expected := `Path("/foo") -> setPath("/") -> "https://www.example.org"`
				got := route.Print(false)
				check(t, got, expected)
			})

			t.Run("pretty", func(t *testing.T) {
				expected := `Path("/foo")
  -> setPath("/")
  -> "https://www.example.org"`
				got := route.Print(true)
				check(t, got, expected)
			})
		})
	})

	t.Run("package level", func(t *testing.T) {
		type packageLevelTest struct {
			title    string
			routes   []*Route
			expected string
		}

		testsBase := []packageLevelTest{{
			title: "single expression (no ID)",
			routes: []*Route{{
				Predicates: []*Predicate{{
					Name: "Path",
					Args: []interface{}{
						"/foo",
					},
				}},
				Filters: []*Filter{{
					Name: "setPath",
					Args: []interface{}{
						"/",
					},
				}},
				Backend: "https://www.example.org",
			}},
		}, {
			title: "single definition (with ID)",
			routes: []*Route{{
				Id: "testRoute1",
				Predicates: []*Predicate{{
					Name: "Path",
					Args: []interface{}{
						"/foo",
					},
				}},
				Filters: []*Filter{{
					Name: "setPath",
					Args: []interface{}{
						"/",
					},
				}},
				Backend: "https://www.example.org",
			}},
		}, {
			title: "empty",
		}, {
			title: "multiple routes",
			routes: []*Route{{
				Id: "testRoute1",
				Predicates: []*Predicate{{
					Name: "Path",
					Args: []interface{}{
						"/foo",
					},
				}},
				Filters: []*Filter{{
					Name: "setPath",
					Args: []interface{}{
						"/",
					},
				}},
				Backend: "https://ww1.example.org",
			}, {
				Id: "testRoute2",
				Predicates: []*Predicate{{
					Name: "Path",
					Args: []interface{}{
						"/bar",
					},
				}},
				Filters: []*Filter{{
					Name: "setPath",
					Args: []interface{}{
						"/",
					},
				}},
				Backend: "https://ww2.example.org",
			}, {
				Id: "testRoute3",
				Predicates: []*Predicate{{
					Name: "Path",
					Args: []interface{}{
						"/baz",
					},
				}},
				Filters: []*Filter{{
					Name: "setPath",
					Args: []interface{}{
						"/",
					},
				}},
				Backend: "https://ww3.example.org",
			}},
		}}

		expectedFlat := []string{
			`Path("/foo") -> setPath("/") -> "https://www.example.org"`,
			`testRoute1: Path("/foo") -> setPath("/") -> "https://www.example.org";`,
			``,
			`testRoute1: Path("/foo") -> setPath("/") -> "https://ww1.example.org";
testRoute2: Path("/bar") -> setPath("/") -> "https://ww2.example.org";
testRoute3: Path("/baz") -> setPath("/") -> "https://ww3.example.org";`,
		}

		expectedPretty := []string{
			`Path("/foo")
  -> setPath("/")
  -> "https://www.example.org"`,
			`testRoute1: Path("/foo")
  -> setPath("/")
  -> "https://www.example.org";`,
			``,
			`testRoute1: Path("/foo")
  -> setPath("/")
  -> "https://ww1.example.org";

testRoute2: Path("/bar")
  -> setPath("/")
  -> "https://ww2.example.org";

testRoute3: Path("/baz")
  -> setPath("/")
  -> "https://ww3.example.org";`,
		}

		makeTests := func(base []packageLevelTest, expected []string) []packageLevelTest {
			tests := make([]packageLevelTest, len(base))
			for i := range base {
				tests[i] = base[i]
				tests[i].expected = expected[i]
			}

			return tests
		}

		testsFlat := makeTests(testsBase, expectedFlat)
		testsPretty := makeTests(testsBase, expectedPretty)

		runTests := func(t *testing.T, tests []packageLevelTest, print func(packageLevelTest) string) {
			for _, test := range tests {
				t.Run(test.title, func(t *testing.T) {
					got := print(test)
					check(t, got, test.expected)
				})
			}
		}

		t.Run("String()", func(t *testing.T) {
			runTests(t, testsFlat, func(test packageLevelTest) string { return String(test.routes...) })
		})

		t.Run("Print()", func(t *testing.T) {
			t.Run("not pretty", func(t *testing.T) {
				runTests(t, testsFlat, func(test packageLevelTest) string {
					return Print(false, test.routes...)
				})
			})

			t.Run("pretty", func(t *testing.T) {
				runTests(t, testsPretty, func(test packageLevelTest) string {
					return Print(true, test.routes...)
				})
			})
		})

		t.Run("Fprint()", func(t *testing.T) {
			fprint := func(pretty bool, routes []*Route) string {
				var buf bytes.Buffer
				Fprint(&buf, pretty, routes...)
				return buf.String()
			}

			t.Run("not pretty", func(t *testing.T) {
				runTests(t, testsFlat, func(test packageLevelTest) string {
					return fprint(false, test.routes)
				})
			})

			t.Run("pretty", func(t *testing.T) {
				runTests(t, testsPretty, func(test packageLevelTest) string {
					return fprint(true, test.routes)
				})
			})
		})
	})
}
