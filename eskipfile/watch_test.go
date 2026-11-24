package eskipfile

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routing"
)

const testWatchFile = "fixtures/watch-test.eskip"

const testWatchFileContent = `
	foo: Path("/foo") -> setPath("/") -> "https://foo.example.org";
	bar: Path("/bar") -> setPath("/") -> "https://bar.example.org";
	baz: Path("/baz") -> setPath("/") -> "https://baz.example.org";
`

const testWatchFileInvalidContent = `
	invalid eskip
`

const testWatchFileUpdatedContent = `
	foo: Path("/foo") -> setPath("/") -> "https://foo.example.org";
	baz: Path("/baz") -> setPath("/") -> "https://baz-new.example.org";
`

type watchTest struct {
	testing *testing.T
	log     *loggingtest.Logger
	file    *WatchClient
	routing *routing.Routing
}

func deleteFile(t *testing.T) {
	err := os.Remove(testWatchFile)
	if err != nil {
		t.Logf("Ignoring %v", err)
	}
}

func createFileWith(content string, t *testing.T) {
	tmpfile, err := os.Create(testWatchFile + ".tmp")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.WriteString(content)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Rename(tmpfile.Name(), testWatchFile)
	if err != nil {
		t.Fatal(err)
	}
}

func createFile(t *testing.T) {
	createFileWith(testWatchFileContent, t)
}

func invalidFile(t *testing.T) {
	createFileWith(testWatchFileInvalidContent, t)
}

func updateFile(t *testing.T) {
	createFileWith(testWatchFileUpdatedContent, t)
}

func initWatchTest(t *testing.T) *watchTest {
	l := loggingtest.New()
	f := Watch(testWatchFile)
	return &watchTest{
		testing: t,
		log:     l,
		file:    f,
		routing: routing.New(routing.Options{
			Log:            l,
			FilterRegistry: builtin.MakeRegistry(),
			DataClients:    []routing.DataClient{f},
			PollTimeout:    15 * time.Millisecond,
		}),
	}
}

func (t *watchTest) testFail(path string) {
	if r, _ := t.routing.Route(&http.Request{URL: &url.URL{Path: path}}); r != nil {
		t.testing.Log("got:     ", r.Id)
		t.testing.Log("expected: nil")
		t.testing.Fatalf("unexpected route received for: %v", path)
	}
}

func (t *watchTest) testSuccess(id, path, backend string) {
	r, _ := t.routing.Route(&http.Request{URL: &url.URL{Path: path}})
	if r == nil {
		t.testing.Fatalf("failed to load route for: %v", path)
		return
	}

	if r.Id != id || r.Backend != backend {
		t.testing.Log("got:     ", r.Id, backend)
		t.testing.Log("expected:", id, r.Backend)
		t.testing.Fatal("unexpected route received")
	}
}

func (t *watchTest) timeoutInitial() {
	defer t.log.Reset()
	err := t.log.WaitFor("route settings applied", 90*time.Millisecond)
	if err == nil {
		t.testing.Fatal("expected timeout, got route settings applied")
	} else if err != loggingtest.ErrWaitTimeout {
		t.testing.Fatalf("unexpected error: %v", err)
	}
}

func (t *watchTest) timeoutAndSucceedInitial() {
	defer t.log.Reset()
	if err := t.log.WaitFor("route settings applied", 90*time.Millisecond); err == nil {
		t.testing.Fatal("unexpected change detected")
	}

	t.testSuccess("foo", "/foo", "https://foo.example.org")
	t.testSuccess("bar", "/bar", "https://bar.example.org")
	t.testSuccess("baz", "/baz", "https://baz.example.org")
}

func (t *watchTest) waitAndFailInitial() {
	defer t.log.Reset()
	if err := t.log.WaitFor("route settings applied", 90*time.Millisecond); err != nil {
		t.testing.Fatal(err)
	}

	t.testFail("/foo")
	t.testFail("/bar")
	t.testFail("/baz")
}

func (t *watchTest) waitAndSucceedInitial() {
	defer t.log.Reset()
	if err := t.log.WaitFor("route settings applied", 90*time.Millisecond); err != nil {
		t.testing.Fatal(err)
	}

	t.testSuccess("foo", "/foo", "https://foo.example.org")
	t.testSuccess("bar", "/bar", "https://bar.example.org")
	t.testSuccess("baz", "/baz", "https://baz.example.org")
}

func (t *watchTest) waitAndSucceedUpdated() {
	defer t.log.Reset()
	if err := t.log.WaitFor("route settings applied", 90*time.Millisecond); err != nil {
		t.testing.Fatal(err)
	}

	t.testSuccess("foo", "/foo", "https://foo.example.org")
	t.testFail("/bar")
	t.testSuccess("baz", "/baz", "https://baz-new.example.org")
}

func (t *watchTest) close() {
	t.log.Close()
	t.file.Close()
	t.routing.Close()
}

func TestWatchInitialFails(t *testing.T) {
	test := initWatchTest(t)
	defer test.close()
	test.timeoutInitial()
}

func TestWatchInitialRecovers(t *testing.T) {
	test := initWatchTest(t)
	defer test.close()
	test.timeoutInitial()
	createFile(t)
	defer deleteFile(t)
	test.waitAndSucceedInitial()
}

func TestWatchUpdateFails(t *testing.T) {
	createFile(t)
	defer deleteFile(t)
	test := initWatchTest(t)
	defer test.close()
	test.waitAndSucceedInitial()
	invalidFile(t)
	test.timeoutAndSucceedInitial()
}

func TestWatchUpdateRecover(t *testing.T) {
	createFile(t)
	defer deleteFile(t)
	test := initWatchTest(t)
	defer test.close()
	test.waitAndSucceedInitial()
	invalidFile(t)
	test.timeoutAndSucceedInitial()
	updateFile(t)
	test.waitAndSucceedUpdated()
}

func TestInitialAndUnchanged(t *testing.T) {
	createFile(t)
	defer deleteFile(t)
	test := initWatchTest(t)
	defer test.close()
	test.waitAndSucceedInitial()
	test.timeoutAndSucceedInitial()
}

func TestInitialAndDeleteFile(t *testing.T) {
	createFile(t)
	defer deleteFile(t)
	test := initWatchTest(t)
	defer test.close()
	test.waitAndSucceedInitial()
	deleteFile(t)
	test.waitAndFailInitial()
}

func TestWatchUpdate(t *testing.T) {
	createFile(t)
	defer deleteFile(t)
	test := initWatchTest(t)
	defer test.close()
	test.waitAndSucceedInitial()
	updateFile(t)
	test.waitAndSucceedUpdated()
}

func BenchmarkWatchLoadUpdate(b *testing.B) {
	f, err := os.CreateTemp(b.TempDir(), "routes*.eskip")
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, os.Remove(f.Name())) })

	const nRoutes = 10_000
	for i := range nRoutes {
		_, err := f.WriteString(fmt.Sprintf(`r%d: Path("/%d") -> "https://foo%d.example.org";`, i, i, i))
		require.NoError(b, err)
	}
	require.NoError(b, f.Close())

	w := Watch(f.Name())
	b.Cleanup(w.Close)

	r, err := w.LoadAll()
	require.Len(b, r, nRoutes)
	require.NoError(b, err)

	r, deletedIds, err := w.LoadUpdate()
	require.Nil(b, r)
	require.Empty(b, deletedIds)
	require.NoError(b, err)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w.LoadUpdate()
	}
}

func BenchmarkDiffStoreRoutes(b *testing.B) {
	const nRoutes = 1000
	var doc string
	for i := range nRoutes {
		doc += fmt.Sprintf(`r%d: Host(/^foo%d\.example\.org$/) && PathSubtree("/%d") && Header("X-Test", "1") -> setRequestHeader("X-Foo", "bar") -> setResponseHeader("X-Baz", "qux") -> <roundRobin, "http://10.0.0.1:8080?zone=eu-central-1a", "http://10.0.0.2:8080?zone=eu-central-1b">;`, i, i, i)
	}

	routesA, err := eskip.Parse(doc)
	require.NoError(b, err)
	routesB, err := eskip.Parse(doc)
	require.NoError(b, err)

	w := &WatchClient{}
	w.storeRoutes(routesA)

	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		if i%2 == 0 {
			_, _ = w.diffStoreRoutes(routesB)
		} else {
			_, _ = w.diffStoreRoutes(routesA)
		}
		i++
	}
}

func TestRoutesEqual(t *testing.T) {
	base := func() *eskip.Route {
		return &eskip.Route{
			Id:            "r1",
			Path:          "/a",
			HostRegexps:   []string{"^example[.]org$"},
			PathRegexps:   []string{"^/a"},
			Method:        "GET",
			Headers:       map[string]string{"X-A": "1"},
			HeaderRegexps: map[string][]string{"X-B": {"^2$"}},
			Predicates:    []*eskip.Predicate{{Name: "Traffic", Args: []interface{}{0.5}}},
			Filters:       []*eskip.Filter{{Name: "status", Args: []interface{}{float64(200)}}},
			BackendType:   eskip.LBBackend,
			LBAlgorithm:   "roundRobin",
			LBEndpoints:   []*eskip.LBEndpoint{{Address: "http://10.0.0.1:8080", Zone: "eu-central-1a"}},
		}
	}

	for _, tc := range []struct {
		name string
		a, b *eskip.Route
		want bool
	}{
		{name: "both nil", a: nil, b: nil, want: true},
		{name: "one nil", a: base(), b: nil, want: false},
		{name: "equal distinct pointers", a: base(), b: base(), want: true},
		{name: "different id", a: base(), b: func() *eskip.Route { r := base(); r.Id = "r2"; return r }(), want: false},
		{name: "different headers", a: base(), b: func() *eskip.Route { r := base(); r.Headers["X-A"] = "2"; return r }(), want: false},
		{name: "different filter args", a: base(), b: func() *eskip.Route { r := base(); r.Filters[0].Args[0] = float64(201); return r }(), want: false},
		{name: "different predicate args", a: base(), b: func() *eskip.Route { r := base(); r.Predicates[0].Args[0] = 0.6; return r }(), want: false},
		{name: "different lb endpoint zone", a: base(), b: func() *eskip.Route { r := base(); r.LBEndpoints[0].Zone = "eu-central-1b"; return r }(), want: false},
		{name: "nil vs empty host regexps", a: func() *eskip.Route { r := base(); r.HostRegexps = nil; return r }(), b: func() *eskip.Route { r := base(); r.HostRegexps = []string{}; return r }(), want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := routesEqual(tc.a, tc.b); got != tc.want {
				t.Errorf("routesEqual() = %v, want %v", got, tc.want)
			}
		})
	}
}
