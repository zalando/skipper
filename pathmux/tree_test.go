package pathmux

import (
	"net/http"
	"strings"
	"testing"
)

type HandlerFunc func(w http.ResponseWriter, r *http.Request, urlParams map[string]string)

func dummyHandler(w http.ResponseWriter, r *http.Request, urlParams map[string]string) {

}

func addPath(t *testing.T, tree *node, path string) {
	t.Logf("Adding path %s", path)
	n, err := tree.addPath(path[1:], nil)
	if err != nil {
		t.Error(err)
	}
	handler := HandlerFunc(func(w http.ResponseWriter, r *http.Request, urlParams map[string]string) {
		urlParams["path"] = path
	})
	n.leafValue = handler
}

var test *testing.T

func testPath(t *testing.T, tree *node, path string, expectPath string, expectedParams map[string]string) {
	if t.Failed() {
		t.FailNow()
	}

	expectCatchAll := strings.Contains(expectPath, "/*")

	t.Log("Testing", path)
	n, paramList, _ := tree.search(path[1:], tm)
	if expectPath != "" && n == nil {
		t.Errorf("No match for %s, expected %s", path, expectPath)
		return
	} else if expectPath == "" && n != nil {
		t.Errorf("Expected no match for %s but got %v with params %v", path, n, expectedParams)
		t.Error("Node and subtree was\n")
		return
	}

	if n == nil {
		return
	}

	if expectCatchAll != n.isCatchAll {
		t.Errorf("For path %s expectCatchAll %v but saw %v", path, expectCatchAll, n.isCatchAll)
	}

	handler, ok := n.leafValue.(HandlerFunc)
	if !ok {
		t.Errorf("Path %s returned node without handler", path)
		t.Error("Node and subtree was\n")
		return
	}

	pathMap := make(map[string]string)
	handler(nil, nil, pathMap)
	matchedPath := pathMap["path"]

	if matchedPath != expectPath {
		t.Errorf("Path %s matched %s, expected %s", path, matchedPath, expectPath)
		t.Error("Node and subtree was\n")
	}

	if expectedParams == nil {
		if len(paramList) != 0 {
			t.Errorf("Path %s expected no parameters, saw %v", path, paramList)
		}
	} else {
		if len(paramList) != len(n.leafWildcardNames) {
			t.Errorf("Got %d params back but node specifies %d",
				len(paramList), len(n.leafWildcardNames))
		}

		params := map[string]string{}
		for i := 0; i < len(paramList); i++ {
			params[n.leafWildcardNames[len(paramList)-i-1]] = paramList[i]
		}
		t.Log("\tGot params", params)

		for key, val := range expectedParams {
			sawVal, ok := params[key]
			if !ok {
				t.Errorf("Path %s matched without key %s", path, key)
			} else if sawVal != val {
				t.Errorf("Path %s expected param %s to be %s, saw %s", path, key, val, sawVal)
			}

			delete(params, key)
		}

		for key, val := range params {
			t.Errorf("Path %s returned unexpected param %s=%s", path, key, val)
		}
	}

}

func checkHandlerNodes(t *testing.T, n *node) {
	hasHandlers := n.leafValue != nil
	hasWildcards := len(n.leafWildcardNames) != 0

	if hasWildcards && !hasHandlers {
		t.Errorf("Node %s has wildcards without handlers", n.path)
	}
}

func TestTree(t *testing.T) {
	test = t
	tree := &node{path: "/"}

	addPath(t, tree, "/")
	addPath(t, tree, "/i")
	addPath(t, tree, "/i/:aaa")
	addPath(t, tree, "/images")
	addPath(t, tree, "/images/abc.jpg")
	addPath(t, tree, "/images/:imgname")
	addPath(t, tree, "/images/*path")
	addPath(t, tree, "/ima")
	addPath(t, tree, "/ima/:par")
	addPath(t, tree, "/images1")
	addPath(t, tree, "/images2")
	addPath(t, tree, "/apples")
	addPath(t, tree, "/app/les")
	addPath(t, tree, "/apples1")
	addPath(t, tree, "/appeasement")
	addPath(t, tree, "/appealing")
	addPath(t, tree, "/date/:year/:month")
	addPath(t, tree, "/date/:year/month")
	addPath(t, tree, "/date/:year/:month/abc")
	addPath(t, tree, "/date/:year/:month/:post")
	addPath(t, tree, "/date/:year/:month/*post")
	addPath(t, tree, "/:page")
	addPath(t, tree, "/:page/:index")
	addPath(t, tree, "/post/:post/page/:page")
	addPath(t, tree, "/plaster")
	addPath(t, tree, "/users/:pk/:related")
	addPath(t, tree, "/users/:id/updatePassword")
	addPath(t, tree, "/:something/abc")
	addPath(t, tree, "/:something/def")

	testPath(t, tree, "/users/abc/updatePassword", "/users/:id/updatePassword",
		map[string]string{"id": "abc"})
	testPath(t, tree, "/users/all/something", "/users/:pk/:related",
		map[string]string{"pk": "all", "related": "something"})

	testPath(t, tree, "/aaa/abc", "/:something/abc",
		map[string]string{"something": "aaa"})
	testPath(t, tree, "/aaa/def", "/:something/def",
		map[string]string{"something": "aaa"})

	testPath(t, tree, "/paper", "/:page",
		map[string]string{"page": "paper"})

	testPath(t, tree, "/", "/", nil)
	testPath(t, tree, "/i", "/i", nil)
	testPath(t, tree, "/images", "/images", nil)
	testPath(t, tree, "/images/abc.jpg", "/images/abc.jpg", nil)
	testPath(t, tree, "/images/something", "/images/:imgname",
		map[string]string{"imgname": "something"})
	testPath(t, tree, "/images/long/path", "/images/*path",
		map[string]string{"path": "long/path"})
	testPath(t, tree, "/images/even/longer/path", "/images/*path",
		map[string]string{"path": "even/longer/path"})
	testPath(t, tree, "/ima", "/ima", nil)
	testPath(t, tree, "/apples", "/apples", nil)
	testPath(t, tree, "/app/les", "/app/les", nil)
	testPath(t, tree, "/abc", "/:page",
		map[string]string{"page": "abc"})
	testPath(t, tree, "/abc/100", "/:page/:index",
		map[string]string{"page": "abc", "index": "100"})
	testPath(t, tree, "/post/a/page/2", "/post/:post/page/:page",
		map[string]string{"post": "a", "page": "2"})
	testPath(t, tree, "/date/2014/5", "/date/:year/:month",
		map[string]string{"year": "2014", "month": "5"})
	testPath(t, tree, "/date/2014/month", "/date/:year/month",
		map[string]string{"year": "2014"})
	testPath(t, tree, "/date/2014/5/abc", "/date/:year/:month/abc",
		map[string]string{"year": "2014", "month": "5"})
	testPath(t, tree, "/date/2014/5/def", "/date/:year/:month/:post",
		map[string]string{"year": "2014", "month": "5", "post": "def"})
	testPath(t, tree, "/date/2014/5/def/hij", "/date/:year/:month/*post",
		map[string]string{"year": "2014", "month": "5", "post": "def/hij"})
	testPath(t, tree, "/date/2014/5/def/hij/", "/date/:year/:month/*post",
		map[string]string{"year": "2014", "month": "5", "post": "def/hij/"})

	testPath(t, tree, "/date/2014/ab%2f", "/date/:year/:month",
		map[string]string{"year": "2014", "month": "ab/"})
	testPath(t, tree, "/post/ab%2fdef/page/2%2f", "/post/:post/page/:page",
		map[string]string{"post": "ab/def", "page": "2/"})

	testPath(t, tree, "/ima/bcd/fgh", "", nil)
	testPath(t, tree, "/date/2014//month", "", nil)
	testPath(t, tree, "/date/2014/05/", "", nil) // Empty catchall should not match
	testPath(t, tree, "/post//abc/page/2", "", nil)
	testPath(t, tree, "/post/abc//page/2", "", nil)
	testPath(t, tree, "/post/abc/page//2", "", nil)
	testPath(t, tree, "//post/abc/page/2", "", nil)
	testPath(t, tree, "//post//abc//page//2", "", nil)

	t.Log("Test retrieval of duplicate paths")
	params := make(map[string]string)
	p := "date/:year/:month/abc"
	n, err := tree.addPath(p, nil)
	if err != nil {
		t.Error(err)
	}
	if n == nil {
		t.Errorf("Duplicate add of %s didn't return a node", p)
	} else {
		handler, ok := n.leafValue.(HandlerFunc)
		matchPath := ""
		if ok {
			handler(nil, nil, params)
			matchPath = params["path"]
		}

		if len(matchPath) < 2 || matchPath[1:] != p {
			t.Errorf("Duplicate add of %s returned node for %s\n", p, matchPath)

		}
	}

	checkHandlerNodes(t, tree)

	test = nil
}

func TestPanics(t *testing.T) {
	sawPanic := false

	addPathPanic := func(p ...string) {
		sawPanic = false
		tree := &node{path: "/"}
		for _, path := range p {
			_, err := tree.addPath(path, nil)
			if err != nil {
				sawPanic = true
			}
		}
	}

	addPathPanic("abc/*path/")
	if !sawPanic {
		t.Error("Expected panic with slash after catch-all")
	}

	addPathPanic("abc/*path/def")
	if !sawPanic {
		t.Error("Expected panic with path segment after catch-all")
	}

	addPathPanic("abc/*path", "abc/*paths")
	if !sawPanic {
		t.Error("Expected panic when adding conflicting catch-alls")
	}

	addPathPanic("abc/ab:cd")
	if !sawPanic {
		t.Error("Expected panic with : in middle of path segment")
	}

	addPathPanic("abc/ab", "abc/ab:cd")
	if !sawPanic {
		t.Error("Expected panic with : in middle of path segment with existing path")
	}

	addPathPanic("abc/ab*cd")
	if !sawPanic {
		t.Error("Expected panic with * in middle of path segment")
	}

	addPathPanic("abc/ab", "abc/ab*cd")
	if !sawPanic {
		t.Error("Expected panic with * in middle of path segment with existing path")
	}

	twoPathPanic := func(first, second string) {
		addPathPanic(first, second)
		if !sawPanic {
			t.Errorf("Expected panic with ambiguous wildcards on paths %s and %s", first, second)
		}
	}

	twoPathPanic("abc/:ab/def/:cd", "abc/:ad/def/:cd")
	twoPathPanic("abc/:ab/def/:cd", "abc/:ab/def/:ef")
	twoPathPanic(":abc", ":def")
	twoPathPanic(":abc/ggg", ":def/ggg")
}

type TestMatcher struct {
	match bool
}

func (fm *TestMatcher) Match(value interface{}) (bool, interface{}) {
	return fm.match, value
}

func TestFalseMatcher(t *testing.T) {
	tree := &Tree{}
	err := tree.Add("/some/path", 1)
	if err != nil {
		t.Error(err)
	}

	v, _, _ := tree.LookupMatcher("/some/path", &TestMatcher{false})

	if v != nil {
		t.Error("failed, no match expected for false matcher")
	}
}

func TestTrueMatcher(t *testing.T) {
	tree := &Tree{}
	err := tree.Add("/some/path", 1)
	if err != nil {
		t.Error(err)
	}

	v, _, _ := tree.LookupMatcher("/some/path", &TestMatcher{true})

	if v == nil {
		t.Error("failed, match expected for true matcher")
	}
}

func TestDefaultMatcher(t *testing.T) {
	tree := &Tree{}
	err := tree.Add("/some/path", 1)
	if err != nil {
		t.Error(err)
	}

	v, _ := tree.Lookup("/some/path")

	if v == nil {
		t.Error("failed, match expected for true matcher")
	}
}

func BenchmarkTreeNullRequest(b *testing.B) {
	b.ReportAllocs()
	tree := &node{path: "/"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.search("", tm)
	}
}

func BenchmarkTreeOneStatic(b *testing.B) {
	b.ReportAllocs()
	tree := &node{path: "/"}
	tree.addPath("abc", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.search("abc", tm)
	}
}

func BenchmarkTreeOneParam(b *testing.B) {
	b.ReportAllocs()
	tree := &node{path: "/"}
	tree.addPath(":abc", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.search("abc", tm)
	}
}
