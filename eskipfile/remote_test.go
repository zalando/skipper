package eskipfile

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/sanity-io/litter"
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
			client, err := RemoteWatch(s.URL, 10, true)
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

			if !reflect.DeepEqual(r, test.expected) {
				t.Error("invalid routes received")
				t.Log("got:     ", litter.Sdump(r))
				t.Log("expected:", litter.Sdump(test.expected))
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

			client, err := RemoteWatch(testValidServer.URL, 10, true)
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

func createTestServer(c string, sc int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(sc)
		io.WriteString(w, c)
	}))
}
