package eskipfile

import (
	"testing"

	"github.com/zalando/skipper/eskip"
)

func TestOpenAndLoadAndParseAll(t *testing.T) {
	f, err := Open("fixtures/test.eskip")
	if err != nil {
		t.Fatal(err)
	}

	ris, err := f.LoadAndParseAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, ri := range ris {
		if ri.ParseError != nil {
			t.Fatal(err)
		}
	}

	check := func(routeInfos []*eskip.RouteInfo, id, path string) {
		for _, ri := range routeInfos {
			if ri.Id == id {
				if ri.Path != path {
					t.Fatalf("Failed to find %q != %q", ri.Path, path)
				}
				return
			}
		}
		t.Fatalf("Failed to find expected route with id %q and path %q", id, path)
	}

	check(ris, "foo", "/foo")
	check(ris, "bar", "/bar")
}
