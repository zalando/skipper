package eskip

import (
	"bytes"
	"fmt"
	"strings"
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
	}, {
		&Route{
			Filters:     []*Filter{{"filter0", []interface{}{"arg"}}},
			BackendType: DynamicBackend},
		`* -> filter0("arg") -> <dynamic>`,
	}, {
		&Route{
			Filters:     []*Filter{{"filter0", []interface{}{`Line 1\r\nLine 2`}}},
			BackendType: DynamicBackend},
		`* -> filter0("Line 1\\r\\nLine 2") -> <dynamic>`,
	}, {
		&Route{
			Filters:     []*Filter{{"filter0", []interface{}{"Line 1\r\nLine 2"}}},
			BackendType: DynamicBackend},
		`* -> filter0("Line 1\r\nLine 2") -> <dynamic>`,
	}, {
		&Route{Method: "GET", BackendType: LBBackend, LBEndpoints: []*LBEndpoint{{Address: "http://127.0.0.1:9997"}}},
		`Method("GET") -> <"http://127.0.0.1:9997">`,
	}, {
		&Route{Method: "GET", LBAlgorithm: "random", BackendType: LBBackend, LBEndpoints: []*LBEndpoint{{Address: "http://127.0.0.1:9997"}}},
		`Method("GET") -> <random, "http://127.0.0.1:9997">`,
	}, {
		&Route{Method: "GET", LBAlgorithm: "random", BackendType: LBBackend, LBEndpoints: []*LBEndpoint{
			{Address: "http://127.0.0.1:9997"},
			{Address: "http://127.0.0.1:9998"},
		}},
		`Method("GET") -> <random, "http://127.0.0.1:9997", "http://127.0.0.1:9998">`,
	}, {
		// test slash escaping
		&Route{Path: `/`, PathRegexps: []string{`/`}, Filters: []*Filter{{"afilter", []interface{}{`/`}}}, BackendType: ShuntBackend},
		`Path("/") && PathRegexp(/\//) -> afilter("/") -> <shunt>`,
	}, {
		// test backslash escaping
		&Route{Path: `\`, PathRegexps: []string{`\`}, Filters: []*Filter{{"afilter", []interface{}{`\`}}}, BackendType: ShuntBackend},
		`Path("\\") && PathRegexp(/\\/) -> afilter("\\") -> <shunt>`,
	}, {
		// test double quote escaping
		&Route{Path: `"`, PathRegexps: []string{`"`}, Filters: []*Filter{{"afilter", []interface{}{`"`}}}, BackendType: ShuntBackend},
		`Path("\"") && PathRegexp(/"/) -> afilter("\"") -> <shunt>`,
	}, {
		// test backslash is escaped before other escape sequences
		&Route{Path: "\\\n\"/", PathRegexps: []string{"\\\n\"/"}, Filters: []*Filter{{"afilter", []interface{}{"\\\n\"/"}}}, BackendType: ShuntBackend},
		`Path("\\\n\"/") && PathRegexp(/\\\n"\//) -> afilter("\\\n\"/") -> <shunt>`,
	}} {
		rstring := item.route.String()
		if rstring != item.string {
			t.Log(rstring)
			t.Log(item.string)

			pos, rstringOut, itemOut := findDiffPos(rstring, item.string)
			t.Error("diff pos:", i, pos, rstringOut, itemOut)
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
		testPrinting(item.route, item.expected, t, i, PrettyPrintInfo{Pretty: false, IndentStr: ""}, false)
	}
}

func TestPrintSortedPredicates(t *testing.T) {
	for i, item := range []struct {
		name     string
		route    string
		expected string
	}{
		{
			"preserves order of regular predicates",
			`routeWithoutDefaultPredicates: True() && Cookie("alpha", "/^enabled$/") -> "https://www.example.org"`,
			`True() && Cookie("alpha", "/^enabled$/") -> "https://www.example.org"`,
		},
		{
			"puts builtin predicate before regular predicates",
			`routeWithDefaultPredicates: True() && Cookie("alpha", "/^enabled$/") && Method("GET") -> "https://www.example.org"`,
			`Method("GET") && True() && Cookie("alpha", "/^enabled$/") -> "https://www.example.org"`,
		},
		{
			"puts Method before Header",
			`routeWithDefaultPredicatesOnly: Header("Accept", "application/json") && Method("GET") -> "https://www.example.org"`,
			`Method("GET") && Header("Accept", "application/json") -> "https://www.example.org"`,
		},
		{
			"sorts Header",
			`routeWithMultipleHeaders: Header("x-frontend-type", "mobile-app") && Header("X-Forwarded-Proto", "http") -> "https://www.example.org"`,
			`Header("X-Forwarded-Proto", "http") && Header("x-frontend-type", "mobile-app") -> "https://www.example.org"`,
		},
		{
			"sorts HeaderRegexp",
			`routeWithMultipleHeadersRegex: HeaderRegexp("User-Agent", /Zelt-(.*)/) && HeaderRegexp("age", /\\d/) -> "https://www.example.org"`,
			`HeaderRegexp("User-Agent", /Zelt-(.*)/) && HeaderRegexp("age", /\\d/) -> "https://www.example.org"`,
		},
		{
			"sorts HeaderRegexp with the same name",
			`routeWithMultipleHeadersRegex: HeaderRegexp("B", /3/) && HeaderRegexp("B", /2/) && HeaderRegexp("A", /1/) -> "https://www.example.org"`,
			`HeaderRegexp("A", /1/) && HeaderRegexp("B", /2/) && HeaderRegexp("B", /3/) -> "https://www.example.org"`,
		},
		{
			"puts Method before Header, Header before HeaderRegexp and sorts",
			`routeComplex: True() && Cookie("alpha", "/^enabled$/") && Method("GET") && Header("x-frontend-type", "mobile-app") && Header("X-Forwarded-Proto", "http") && HeaderRegexp("User-Agent", /Zelt-(.*)/) && HeaderRegexp("age", /\\d/) -> "https://www.example.org"`,
			`Method("GET") && Header("X-Forwarded-Proto", "http") && Header("x-frontend-type", "mobile-app") && HeaderRegexp("User-Agent", /Zelt-(.*)/) && HeaderRegexp("age", /\\d/) && True() && Cookie("alpha", "/^enabled$/") -> "https://www.example.org"`,
		},
	} {
		t.Run(item.name, func(t *testing.T) {
			testPrinting(item.route, item.expected, t, i, PrettyPrintInfo{Pretty: false, IndentStr: ""}, false)
		})
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
		testPrinting(item.route, item.expected, t, i, PrettyPrintInfo{Pretty: true, IndentStr: "  "}, false)
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
		t, 0, PrettyPrintInfo{Pretty: true, IndentStr: "  "}, true)
}

func TestPrintMultiRouteNonPretty(t *testing.T) {
	testPrinting(`route1: Method("GET") -> filter("expression") -> <shunt>;`+"\n"+
		`route2: Path("/some/path") -> "https://www.example.org"`,
		`route1: Method("GET") -> filter("expression") -> <shunt>;`+"\n"+
			`route2: Path("/some/path") -> "https://www.example.org";`,
		t, 0, PrettyPrintInfo{Pretty: false, IndentStr: ""}, true)
}

func testPrinting(routestr string, expected string, t *testing.T, i int, prettyPrintInfo PrettyPrintInfo, multi bool) {
	routes, err := Parse(routestr)
	if err != nil {
		t.Fatal(err)
	}
	var printedRoute string

	if multi {
		printedRoute = Print(prettyPrintInfo, routes...)
	} else {
		printedRoute = routes[0].Print(prettyPrintInfo)
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
	_ = testDoc(t, doc)
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
				got := route.Print(PrettyPrintInfo{Pretty: false, IndentStr: ""})
				check(t, got, expected)
			})

			t.Run("pretty", func(t *testing.T) {
				expected := `Path("/foo")
  -> setPath("/")
  -> "https://www.example.org"`
				got := route.Print(PrettyPrintInfo{Pretty: true, IndentStr: "  "})
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
					return Print(PrettyPrintInfo{Pretty: false, IndentStr: ""}, test.routes...)
				})
			})

			t.Run("pretty", func(t *testing.T) {
				runTests(t, testsPretty, func(test packageLevelTest) string {
					return Print(PrettyPrintInfo{Pretty: true, IndentStr: "  "}, test.routes...)
				})
			})
		})

		t.Run("Fprint()", func(t *testing.T) {
			fprint := func(pretty PrettyPrintInfo, routes []*Route) string {
				var buf bytes.Buffer
				Fprint(&buf, pretty, routes...)
				return buf.String()
			}

			t.Run("not pretty", func(t *testing.T) {
				runTests(t, testsFlat, func(test packageLevelTest) string {
					return fprint(PrettyPrintInfo{Pretty: false, IndentStr: ""}, test.routes)
				})
			})

			t.Run("pretty", func(t *testing.T) {
				runTests(t, testsPretty, func(test packageLevelTest) string {
					return fprint(PrettyPrintInfo{Pretty: true, IndentStr: "  "}, test.routes)
				})
			})
		})
	})
}

func BenchmarkLBBackendString(b *testing.B) {
	doc := fmt.Sprintf("* -> <random %s>", strings.Repeat(`, "http://127.0.0.1:9997"`, 50))
	routes, err := Parse(doc)
	if err != nil {
		b.Fatal(err)
	}
	route := routes[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = route.String()
	}
}
