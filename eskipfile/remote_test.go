package eskipfile

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
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

			if result != test.expected {
				t.Error("isRemoteFile failed")
				t.Log(test)
			}
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
		expected: []*eskip.Route{{
			Id:   "VALID",
			Path: "/",
			Filters: []*eskip.Filter{{
				Name: "setPath",
				Args: []interface{}{
					"/homepage/",
				},
			}},
			BackendType: eskip.NetworkBackend,
			Shunt:       false,
			Backend:     "https://example.com/",
		}},
	},
	} {
		s := createTestServer(test.routeContent, test.routeStatusCode)
		defer s.Close()

		t.Run(test.title, func(t *testing.T) {
			options := &RemoteWatchOptions{RemoteFile: s.URL, Threshold: 10, Verbose: true, FailOnStartup: true}
			client, err := RemoteWatch(options)
			defer func() {
				c, ok := client.(*remoteEskipFile)
				if ok {
					c.Close()
				}
			}()
			if err == nil && test.fail {
				t.Error("failed to fail")
				return
			} else if err != nil && !test.fail {
				t.Error(err)
				return
			} else if test.fail {
				return
			}

			r, err := client.LoadAll()
			if err != nil {
				t.Error(err)
				return
			}

			if len(r) == 0 {
				r = nil
			}

			if !cmp.Equal(r, test.expected) {
				t.Errorf("invalid routes received\n%s", cmp.Diff(r, test.expected))
			}
		})
	}
}

func TestLoadAllAndUpdate(t *testing.T) {
	for _, test := range []struct {
		title               string
		validRouteContent   string
		invalidRouteContent string
		expectedToFail      bool
		fail                bool
	}{{
		title:               "Download invalid update and all routes returns routes nil",
		validRouteContent:   fmt.Sprintf("VALID: %v;", routeBody),
		invalidRouteContent: fmt.Sprintf("MISSING_SEMICOLON: %v\nVALID: %v;", routeBody, routeBody),
		expectedToFail:      true,
	},
	} {
		t.Run(test.title, func(t *testing.T) {
			testValidServer := createTestServer(test.validRouteContent, 200)
			defer testValidServer.Close()

			options := &RemoteWatchOptions{RemoteFile: testValidServer.URL, Threshold: 10, Verbose: true, FailOnStartup: true}
			client, err := RemoteWatch(options)
			if err == nil && test.fail {
				t.Error("failed to fail")
				return
			} else if err != nil && !test.fail {
				t.Error(err)
				return
			} else if test.fail {
				return
			}

			testInvalidServer := createTestServer(test.invalidRouteContent, 200)
			defer testInvalidServer.Close()

			client.(*remoteEskipFile).remotePath = testInvalidServer.URL
			_, _, err = client.LoadUpdate()
			if test.expectedToFail && err == nil {
				t.Error(err)
				return
			}

			_, err = client.LoadAll()
			if test.expectedToFail && err == nil {
				t.Error(err)
				return
			}
		})
	}
}

func TestHTTPTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()
	_, err := RemoteWatch(&RemoteWatchOptions{RemoteFile: server.URL, HTTPTimeout: 1 * time.Second, FailOnStartup: true})
	if err, ok := err.(net.Error); !ok || !err.Timeout() {
		t.Errorf("got %v, expected net.Error with timeout", err)
	}
}

func createTestServer(c string, sc int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(sc)
		io.WriteString(w, c)
	}))
}
