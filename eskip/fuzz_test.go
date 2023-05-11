package eskip

import (
	"testing"
)

func FuzzEskipParse(f *testing.F) {
	for _, tc := range []string{
		`Path("newlines") -> blockContentHex("\x00\xab\xf5\xff\x47") -> <shunt>`,
		"Path(`newlines`) -> inlineContent(`foo`) -> <shunt>",
		`PathRegexp(/\.html$/) && Header("Accept", "text/html") -> modPath(/\.html$/, ".jsx") -> requestHeader("X-Type", "page") -> "https://render.example.com"`,
		`route1: Path("/some/path") -> "https://backend-0.example.com";
		route2: Path("/some/other/path") -> fixPath() -> "https://backend-1.example.com";
		route3:
		            Method("POST") && Path("/api") ->
		            requestHeader("X-Type", "ajax-post") ->
		            "https://api.example.com";
		catchAll: * -> "https://www.example.org";
		catchAllWithCustom: * && Custom() -> "https://www.example.org";`,
	} {
		f.Add(tc)
	}

	f.Fuzz(func(t *testing.T, orig string) {
		if r := recover(); r != nil {
			t.Fatalf("Failed to parse %q: %v", orig, r)
		}
		parse(orig)
	})
}
