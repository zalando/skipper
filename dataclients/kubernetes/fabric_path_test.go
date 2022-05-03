package kubernetes

import "testing"

func TestFabricPathStrToPredicate(t *testing.T) {
	for _, tc := range []struct {
		name       string
		fabricPath string
		wantPred   string
	}{
		{"literal path", "/foo/bar", `Path("/foo/bar")`},
		{"last star wildcard", "/foo/bar/*", `Path("/foo/bar/:id")`},
		{"one star wildcard", "/foo/*/bar", `Path("/foo/:id/bar")`},
		{"two star wildcards", "/foo/*/bar/*/baz", `Path("/foo/:id/bar/:id/baz")`},
		{"curly wildcard", "/api/resources/{name}", `Path("/api/resources/:name")`},
		{"two curly wildcards", "/foo/{name}/bar/{subname}", `Path("/foo/:name/bar/:subname")`},
		{"double star wildcard", "/api/resources/**", `Path("/api/resources/**")`},
		// mz: Poorly documented behaviors
		{"named last star wildcard", "/foo/bar/*bar", `Path("/foo/bar/:bar")`},
		{"named star wildcard", "/foo/*bar/baz", `Path("/foo/:bar/baz")`},
		{"bare double star wildcard", "/**", `PathSubtree("/")`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := fabricPathStrToPredicate(tc.fabricPath).String()
			want := tc.wantPred
			if got != want {
				t.Errorf(`%v != %v`, got, tc.wantPred)
			}
		})
	}
}
