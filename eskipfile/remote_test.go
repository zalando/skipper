package eskipfile

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/logging/loggingtest"
	"github.com/zalando/skipper/routing"
)

const routeBody string = `Path("/") -> setPath("/homepage/") -> "https://example.com/"`

type gatedRemoteWatch struct {
	client        routing.DataClient
	updateStarted chan struct{}
	releaseUpdate <-chan struct{}
	updateErr     chan error
	updateOnce    sync.Once
}

func (g *gatedRemoteWatch) LoadAll() ([]*eskip.Route, error) {
	return g.client.LoadAll()
}

func (g *gatedRemoteWatch) LoadUpdate() ([]*eskip.Route, []string, error) {
	first := false
	g.updateOnce.Do(func() { first = true })
	if !first {
		return nil, nil, nil
	}

	g.updateStarted <- struct{}{}
	<-g.releaseUpdate
	routes, deleted, err := g.client.LoadUpdate()
	g.updateErr <- err
	return routes, deleted, err
}

func TestIsRemoteFile(t *testing.T) {
	for _, test := range []struct {
		title    string
		file     string
		expected bool
	}{
		{
			title:    "HTTP file",
			file:     "http://example.com/foo",
			expected: true,
		},
		{
			title:    "HTTPS file",
			file:     "https://example.com/foo",
			expected: true,
		},
		{
			title:    "Windows file",
			file:     "c:\folder\foo",
			expected: false,
		},
		{
			title:    "UNIX file",
			file:     "/var/tmp/foo",
			expected: false,
		},
	} {
		t.Run(test.title, func(t *testing.T) {
			result := isFileRemote(test.file)
			assert.Equal(t, result, test.expected)
		})
	}
}

func TestLoadAll(t *testing.T) {
	for _, test := range []struct {
		title           string
		routeContent    string
		routeStatusCode int
		expected        []*eskip.Route
		failOnStartup   bool
		fail            bool
	}{
		{
			title:           "Download not existing remote file fails in NewRemoteEskipFile fail on startup",
			routeContent:    "",
			routeStatusCode: 404,
			failOnStartup:   true,
			fail:            true,
		},
		{
			title:           "Download not existing remote file fails in NewRemoteEskipFile",
			routeContent:    "",
			routeStatusCode: 404,
			failOnStartup:   false,
			fail:            false,
		},
		{
			title:           "Download valid remote file",
			routeContent:    fmt.Sprintf("VALID: %v;", routeBody),
			routeStatusCode: 200,
			failOnStartup:   true,
			expected:        eskip.MustParse(fmt.Sprintf("VALID: %v;", routeBody)),
		},
	} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(test.routeStatusCode)
			io.WriteString(w, test.routeContent)
		}))
		defer ts.Close()

		t.Run(test.title, func(t *testing.T) {
			options := &RemoteWatchOptions{RemoteFile: ts.URL, Threshold: 10, Verbose: true, FailOnStartup: test.failOnStartup}
			client, err := RemoteWatch(options)

			if test.fail {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			defer client.(*remoteEskipFile).Close()

			r, err := client.LoadAll()
			require.NoError(t, err)

			assert.Equal(t, r, test.expected)
		})
	}
}

func TestLoadAllAndUpdate(t *testing.T) {
	for _, test := range []struct {
		title          string
		content        string
		contentUpdated string
		expectedToFail bool
		fail           bool
	}{{
		title:          "Download invalid update and all routes returns routes nil",
		content:        fmt.Sprintf("VALID: %v;", routeBody),
		contentUpdated: fmt.Sprintf("MISSING_SEMICOLON: %v\nVALID: %v;", routeBody, routeBody),
		expectedToFail: true,
	}, {
		title:          "Download valid update and all routes returns routes",
		content:        fmt.Sprintf("VALID: %v;", routeBody),
		contentUpdated: fmt.Sprintf("DIFFERENT_ID: %v;\nVALID: %v;", routeBody, routeBody),
		expectedToFail: false,
	},
	} {
		t.Run(test.title, func(t *testing.T) {
			ch := make(chan string, 1)
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				routeString := <-ch
				t.Logf("server routes: %v", routeString)
				io.WriteString(w, routeString)
			}))

			defer ts.Close()

			options := &RemoteWatchOptions{RemoteFile: ts.URL, Threshold: 10, Verbose: true, FailOnStartup: true}
			ch <- test.content
			client, err := RemoteWatch(options)
			if test.fail {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			defer client.(*remoteEskipFile).Close()

			t.Logf("local path is: %s", client.(*remoteEskipFile).localPath)

			ch <- test.content
			r, err := client.LoadAll()
			require.NoError(t, err)

			expected := eskip.MustParse(test.content)

			assert.Equal(t, r, expected)

			ch <- test.contentUpdated
			r, _, err = client.LoadUpdate()
			t.Logf("routes returned: %+v", r)

			if test.expectedToFail {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			expected = eskip.MustParse(fmt.Sprintf("DIFFERENT_ID: %v;", routeBody))

			assert.Equal(t, r, expected)
		})
	}
}

func TestRemoteButLocalURL(t *testing.T) {
	client, err := RemoteWatch(&RemoteWatchOptions{RemoteFile: "fixtures/test.eskip", FailOnStartup: true})
	defer client.(*WatchClient).Close()
	require.NoError(t, err)
	r, err := client.LoadAll()
	require.NoError(t, err)
	assert.Equal(t, 2, len(r))
}

func TestRemoteLoadAllFail(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch count {
		case 0:
			// RemoteWatch() preloads
			count++
			b, err := os.ReadFile("fixtures/test.eskip")
			if err != nil {
				t.Fatalf("Failed to read file: %v", err)
			}
			w.WriteHeader(200)
			w.Write(b)
		default:
			// LoadAll should fail
			w.WriteHeader(500)
			w.Write([]byte(http.StatusText(500)))
		}
	}))
	defer server.Close()

	client, err := RemoteWatch(&RemoteWatchOptions{RemoteFile: server.URL, FailOnStartup: true})
	defer client.(*remoteEskipFile).Close()
	require.NoError(t, err)

	r, err := client.LoadAll()
	require.Error(t, err)
	assert.Equal(t, 0, len(r))

	r, delR, err := client.LoadUpdate()
	require.Error(t, err)
	assert.Equal(t, 0, len(r))
	assert.Equal(t, 0, len(delR))
}

func TestRemoteInterruptedDownloadPreservesRoutes(t *testing.T) {
	const (
		etag1 = "routes-v1"
		etag2 = "routes-v2"
	)
	initialRoutes := `A: Path("/a") -> "https://a.example.org"; B: Path("/b") -> "https://b.example.org";`
	prefix := `A: Path("/a") -> "https://a.example.org";`
	requestCount := atomic.Int32{}
	requestEtags := make(chan string, 4)
	recoveryStarted := make(chan struct{})
	recoveryRelease := make(chan struct{})
	var recoveryOnce sync.Once
	releaseRecovery := func() { recoveryOnce.Do(func() { close(recoveryRelease) }) }

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestEtags <- r.Header.Get("If-None-Match")
		switch requestCount.Add(1) {
		case 1:
			w.Header().Set("ETag", etag1)
			_, _ = io.WriteString(w, initialRoutes)
		case 2:
			w.WriteHeader(http.StatusNotModified)
		case 3:
			w.Header().Set("ETag", etag2)
			w.Header().Set("Content-Length", fmt.Sprint(len(initialRoutes)))
			_, _ = io.WriteString(w, prefix)
			w.(http.Flusher).Flush()
		case 4:
			close(recoveryStarted)
			<-recoveryRelease
			w.WriteHeader(http.StatusNotModified)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client, err := RemoteWatch(&RemoteWatchOptions{RemoteFile: server.URL, FailOnStartup: true})
	require.NoError(t, err)
	remote := client.(*remoteEskipFile)
	defer remote.Close()

	releaseUpdate := make(chan struct{})
	var releaseOnce sync.Once
	releaseUpdateGate := func() { releaseOnce.Do(func() { close(releaseUpdate) }) }
	gate := &gatedRemoteWatch{
		client:        client,
		updateStarted: make(chan struct{}, 1),
		releaseUpdate: releaseUpdate,
		updateErr:     make(chan error, 1),
	}
	logger := loggingtest.New()
	defer logger.Close()
	rt := routing.New(routing.Options{
		FilterRegistry: builtin.MakeRegistry(),
		DataClients:    []routing.DataClient{gate},
		PollTimeout:    time.Millisecond,
		Log:            logger,
	})
	defer rt.Close()
	defer releaseRecovery()
	defer releaseUpdateGate()

	require.NoError(t, logger.WaitFor("route settings applied", 5*time.Second))
	select {
	case <-gate.updateStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("routing did not start a route update")
	}

	checkRoutes := func() {
		for _, tc := range []struct {
			id   string
			path string
		}{
			{id: "A", path: "/a"},
			{id: "B", path: "/b"},
		} {
			route, _ := rt.Route(&http.Request{URL: &url.URL{Path: tc.path}})
			require.NotNil(t, route, "route %s is not active", tc.id)
			require.Equal(t, tc.id, route.Id)
		}
	}

	checkRoutes()
	releaseUpdateGate()

	select {
	case err := <-gate.updateErr:
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	case <-time.After(5 * time.Second):
		t.Fatal("remote route update did not return")
	}
	select {
	case <-recoveryStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("recovery LoadAll did not reach the 304 response")
	}

	for _, want := range []string{"", etag1, etag1, etag2} {
		select {
		case got := <-requestEtags:
			require.Equal(t, want, got)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for the expected HTTP request")
		}
	}
	require.Equal(t, int32(4), requestCount.Load())
	checkRoutes()

	releaseRecovery()
	require.NoError(t, logger.WaitForN("route settings applied", 2, 5*time.Second))
	checkRoutes()
}

func TestHTTPTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()
	client, err := RemoteWatch(&RemoteWatchOptions{RemoteFile: server.URL, HTTPTimeout: 1 * time.Second, FailOnStartup: true})
	if err == nil {
		defer client.(*remoteEskipFile).Close()
	}

	if err, ok := err.(net.Error); !ok || !err.Timeout() {
		t.Errorf("got %v, expected net.Error with timeout", err)
	}
}

func TestRoutesCaching(t *testing.T) {
	count200s := atomic.Int32{}
	count304s := atomic.Int32{}
	expected := eskip.MustParse(fmt.Sprintf("VALID: %v;", routeBody))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if noneMatch := r.Header.Get("If-None-Match"); noneMatch == "test-etag" {
			t.Logf("request matches etag: %s", noneMatch)
			w.WriteHeader(http.StatusNotModified)
			count304s.Add(1)
		} else {
			w.Header().Set("ETag", "test-etag")
			io.WriteString(w, fmt.Sprintf("VALID: %v;", routeBody))
			count200s.Add(1)
		}
	}))
	defer server.Close()

	options := &RemoteWatchOptions{RemoteFile: server.URL, Threshold: 10, Verbose: true, FailOnStartup: true}
	client, err := RemoteWatch(options) // First load done with initialization because of FailOnStartup

	require.NoError(t, err)

	defer client.(*remoteEskipFile).Close()

	r, err := client.LoadAll()

	t.Logf("uncached responses received: %d", count200s.Load())
	assert.Equal(t, int32(1), count200s.Load())
	t.Logf("cached responses received: %d", count304s.Load())
	assert.Equal(t, int32(1), count304s.Load())

	require.NoError(t, err)

	assert.Equal(t, r, expected)

}

func TestRoutesCachingWrongEtag(t *testing.T) {
	alternate := atomic.Int32{}
	count200s := atomic.Int32{}
	count304s := atomic.Int32{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedEtag := "test-etag"
		if alternate.Load()%2 == 0 {
			expectedEtag = "different-etag"
		}
		if noneMatch := r.Header.Get("If-None-Match"); noneMatch == expectedEtag {
			w.WriteHeader(http.StatusNotModified)
			count304s.Add(1)
		} else {
			if alternate.Load()%2 == 0 {
				w.Header().Set("ETag", "different-etag")
				io.WriteString(w, fmt.Sprintf("different: %v;", routeBody))
			} else {
				w.Header().Set("ETag", "test-etag")
				io.WriteString(w, fmt.Sprintf("VALID: %v;", routeBody))
			}
			count200s.Add(1)
		}
		alternate.Add(1)
	}))
	defer ts.Close()

	options := &RemoteWatchOptions{RemoteFile: ts.URL, Threshold: 10, Verbose: true, FailOnStartup: true}
	client, err := RemoteWatch(options)
	require.NoError(t, err)

	defer client.(*remoteEskipFile).Close()

	r, err := client.LoadAll()

	require.NoError(t, err)

	t.Logf("uncached responses received: %d", count200s.Load())
	assert.Equal(t, int32(2), count200s.Load())
	t.Logf("cached responses received: %d", count304s.Load())
	assert.Equal(t, int32(0), count304s.Load())

	expected := eskip.MustParse(fmt.Sprintf("different: %v;", routeBody))

	t.Logf("routes returned: %s", r[0].Id)
	t.Logf("routes expected: %s", expected[0].Id)

	assert.NotEqual(t, r, expected)

	r, err = client.LoadAll()

	require.NoError(t, err)

	t.Logf("uncached responses received: %d", count200s.Load())
	assert.Equal(t, int32(3), count200s.Load())
	t.Logf("cached responses received: %d", count304s.Load())
	assert.Equal(t, int32(0), count304s.Load())

	expected = eskip.MustParse(fmt.Sprintf("VALID: %v;", routeBody))

	t.Logf("routes returned: %s", r[0].Id)
	t.Logf("routes expected: %s", expected[0].Id)

	assert.NotEqual(t, r, expected)

}
