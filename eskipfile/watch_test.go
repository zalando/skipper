package eskipfile

import (
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

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

func deleteFile() {
	os.RemoveAll(testWatchFile)
}

func createFileWith(content string) {
	f, err := os.Create(testWatchFile)
	if err != nil {
		return
	}

	defer f.Close()
	f.Write([]byte(content))
}

func createFile() {
	createFileWith(testWatchFileContent)
}

func invalidFile() {
	createFileWith(testWatchFileInvalidContent)
}

func updateFile() {
	createFileWith(testWatchFileUpdatedContent)
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
			PollTimeout:    6 * time.Millisecond,
		}),
	}
}

func (t *watchTest) testFail(path string) {
	if r, _ := t.routing.Route(&http.Request{URL: &url.URL{Path: path}}); r != nil {
		t.testing.Error("unexpected route received for:", path)
		t.testing.Log("got:     ", r.Id)
		t.testing.Log("expected: nil")
		t.testing.FailNow()
	}
}

func (t *watchTest) testSuccess(id, path, backend string) {
	r, _ := t.routing.Route(&http.Request{URL: &url.URL{Path: path}})
	if r == nil {
		t.testing.Error("failed to load route for:", path)
		t.testing.FailNow()
		return
	}

	if r.Id != id || r.Backend != backend {
		t.testing.Error("unexpected route received")
		t.testing.Log("got:     ", r.Id, backend)
		t.testing.Log("expected:", id, r.Backend)
		t.testing.FailNow()
	}
}

func (t *watchTest) timeoutOrFailInitial() {
	if t.testing.Failed() {
		return
	}

	defer t.log.Reset()
	if err := t.log.WaitFor("route settings applied", 30*time.Millisecond); err != nil {
		// timeout is also good, the routing handles its own
		return
	}

	t.testFail("/foo")
	t.testFail("/bar")
	t.testFail("/baz")
}

func (t *watchTest) timeoutAndSucceedInitial() {
	if t.testing.Failed() {
		return
	}

	defer t.log.Reset()
	if err := t.log.WaitFor("route settings applied", 30*time.Millisecond); err == nil {
		t.testing.Error("unexpected change detected")
	}

	t.testSuccess("foo", "/foo", "https://foo.example.org")
	t.testSuccess("bar", "/bar", "https://bar.example.org")
	t.testSuccess("baz", "/baz", "https://baz.example.org")
}

func (t *watchTest) waitAndFailInitial() {
	if t.testing.Failed() {
		return
	}

	defer t.log.Reset()
	if err := t.log.WaitFor("route settings applied", 30*time.Millisecond); err != nil {
		t.testing.Fatal(err)
	}

	t.testFail("/foo")
	t.testFail("/bar")
	t.testFail("/baz")
}

func (t *watchTest) waitAndSucceedInitial() {
	if t.testing.Failed() {
		return
	}

	defer t.log.Reset()
	if err := t.log.WaitFor("route settings applied", 30*time.Millisecond); err != nil {
		t.testing.Fatal(err)
	}

	t.testSuccess("foo", "/foo", "https://foo.example.org")
	t.testSuccess("bar", "/bar", "https://bar.example.org")
	t.testSuccess("baz", "/baz", "https://baz.example.org")
}

func (t *watchTest) waitAndSucceedUpdated() {
	if t.testing.Failed() {
		return
	}

	defer t.log.Reset()
	if err := t.log.WaitFor("route settings applied", 30*time.Millisecond); err != nil {
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
	deleteFile()
	test := initWatchTest(t)
	defer test.close()
	test.timeoutOrFailInitial()
}

func TestWatchInitialRecovers(t *testing.T) {
	deleteFile()
	test := initWatchTest(t)
	defer test.close()
	test.timeoutOrFailInitial()
	createFile()
	defer deleteFile()
	test.waitAndSucceedInitial()
}

func TestWatchUpdateFails(t *testing.T) {
	createFile()
	defer deleteFile()
	test := initWatchTest(t)
	defer test.close()
	test.waitAndSucceedInitial()
	invalidFile()
	test.timeoutAndSucceedInitial()
}

func TestWatchUpdateRecover(t *testing.T) {
	createFile()
	defer deleteFile()
	test := initWatchTest(t)
	defer test.close()
	test.waitAndSucceedInitial()
	invalidFile()
	test.timeoutAndSucceedInitial()
	updateFile()
	test.waitAndSucceedUpdated()
}

func TestInitialAndUnchanged(t *testing.T) {
	createFile()
	defer deleteFile()
	test := initWatchTest(t)
	defer test.close()
	test.waitAndSucceedInitial()
	test.timeoutAndSucceedInitial()
}

func TestInitialAndDeleteFile(t *testing.T) {
	createFile()
	defer deleteFile()
	test := initWatchTest(t)
	defer test.close()
	test.waitAndSucceedInitial()
	deleteFile()
	test.waitAndFailInitial()
}

func TestWatchUpdate(t *testing.T) {
	createFile()
	defer deleteFile()
	test := initWatchTest(t)
	defer test.close()
	test.waitAndSucceedInitial()
	updateFile()
	test.waitAndSucceedUpdated()
}
