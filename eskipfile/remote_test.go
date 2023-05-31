package eskipfile

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
)

const routeBody string = `Path("/") -> setPath("/homepage/") -> "https://example.com/"`

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
		fail            bool
	}{{
		title:           "Download not existing remote file fails in NewRemoteEskipFile",
		routeContent:    "",
		routeStatusCode: 404,
		fail:            true,
	}, {
		title:           "Download valid remote file",
		routeContent:    fmt.Sprintf("VALID: %v;", routeBody),
		routeStatusCode: 200,
		expected:        eskip.MustParse(fmt.Sprintf("VALID: %v;", routeBody)),
	},
	} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(test.routeStatusCode)
			io.WriteString(w, test.routeContent)
		}))
		defer ts.Close()

		t.Run(test.title, func(t *testing.T) {
			options := &RemoteWatchOptions{RemoteFile: ts.URL, Threshold: 10, Verbose: true, FailOnStartup: true}
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
