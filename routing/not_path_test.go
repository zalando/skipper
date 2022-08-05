package routing

import (
	"net/http"
	"testing"
)

func TestX(t *testing.T) {
	pathSpec := NewNotPath()
	predicate, err := pathSpec.Create([]interface{}{"/some/path"})
	if err != nil {
		t.Errorf("failed to create predicate: %v", err)
	}

	match := predicate.Match(&http.Request{RequestURI: "/other/path"})
	if match {
		t.Error("should not match")
	}
}

func Test(t *testing.T) {
	tests := []struct {
		name                string
		predicatePath       string
		providedPath        string
		predicateSuccessful bool
	}{
		{
			name:                "different path, does not match",
			predicatePath:       "/some/path",
			providedPath:        "/other/path",
			predicateSuccessful: true,
		},
		{
			name:                "same path, exact match",
			predicatePath:       "/some/path",
			providedPath:        "/some/path",
			predicateSuccessful: false,
		},
		{
			name:                "path with single matching variable",
			predicatePath:       "/some/:variable",
			providedPath:        "/some/path",
			predicateSuccessful: false,
		},
		{
			name:                "path with single variable, more segments provided, does not match",
			predicatePath:       "/some/:variable",
			providedPath:        "/some/path/with/more",
			predicateSuccessful: true,
		},
		{
			name:                "path with wildcard variable, only root provided, does not match",
			predicatePath:       "/some/*variables",
			providedPath:        "/some",
			predicateSuccessful: true,
		},
		{
			name:                "path with wildcard variable, single segment provided, matches",
			predicatePath:       "/some/*variables",
			providedPath:        "/some/path",
			predicateSuccessful: false,
		},
		{
			name:                "path with wildcard variable, multiple segments provided, matches",
			predicatePath:       "/some/*variables",
			providedPath:        "/some/path/with/more",
			predicateSuccessful: false,
		},
		{
			name:                "path with double star wildcard,only root provided, matches",
			predicatePath:       "/some/**",
			providedPath:        "/some",
			predicateSuccessful: true,
		},
		{
			name:                "path with double star wildcard, single segment provided, matches",
			predicatePath:       "/some/**",
			providedPath:        "/some/path",
			predicateSuccessful: false,
		},
		{
			name:                "path with double star wildcard, multiple segments provided, matches",
			predicatePath:       "/some/**",
			providedPath:        "/some/path/with/more",
			predicateSuccessful: false,
		},
		{
			name:                "same path",
			predicatePath:       "/some/:variable/:another",
			providedPath:        "/some/path",
			predicateSuccessful: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pathSpec := NewNotPath()
			predicate, err := pathSpec.Create([]interface{}{test.predicatePath})
			if err != nil {
				t.Errorf("failed to create predicate: %v", err)
			}

			match := predicate.Match(&http.Request{RequestURI: test.providedPath})
			if match != test.predicateSuccessful {
				t.Errorf("predicate expected %v (configured with %s), but got %v (with path %v)", test.predicateSuccessful, test.predicatePath, match, test.providedPath)
			}
		})
	}
}
