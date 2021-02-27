package eskipfile

import (
	"reflect"
	"testing"

	"github.com/sanity-io/litter"
	"github.com/zalando/skipper/eskip"
)

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
		title     string
		remoteURL string
		expected  []*eskip.Route
		fail      bool
	}{{
		title:     "Download not existing remote file fails in NewRemoteEskipFile",
		remoteURL: "https://s3-eu-west-1.amazonaws.com/vprouting/tests/routes-not-existing-file.eskip",
		fail:      true,
	}, {
		title:     "Download valid remote file",
		remoteURL: "https://s3-eu-west-1.amazonaws.com/vprouting/tests/routes.eskip",
		expected: []*eskip.Route{{
			Id:   "VISTAPRINT_HOME_EN_IE",
			Path: "/",
			HostRegexps: []string{
				"vistaprint",
			},
			Filters: []*eskip.Filter{{
				Name: "setPath",
				Args: []interface{}{
					"/homepage/en-ie/",
				},
			}},
			BackendType: eskip.NetworkBackend,
			Shunt:       false,
			Backend:     "https://sandbox.ssp.merch.vpsvc.com/",
		}},
	},
	} {
		t.Run(test.title, func(t *testing.T) {
			client, err := RemoteWatch(test.remoteURL, 10, true)
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
		title            string
		remoteURL        string
		remoteURLInvalid string
		expectedToFail   bool
		fail             bool
	}{{
		title:            "Download invalid update and all routes returns routes nil",
		remoteURL:        "https://s3-eu-west-1.amazonaws.com/vprouting/tests/routes.eskip",
		remoteURLInvalid: "https://s3-eu-west-1.amazonaws.com/vprouting/tests/routes-invalid.eskip",
		expectedToFail:   true,
	},
	} {
		t.Run(test.title, func(t *testing.T) {
			client, err := RemoteWatch(test.remoteURL, 10, true)
			if err == nil && test.fail {
				t.Error("failed to fail")
				return
			} else if err != nil && !test.fail {
				t.Error(err)
				return
			} else if test.fail {
				return
			}

			client.(*remoteEskipFile).remotePath = test.remoteURLInvalid
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
