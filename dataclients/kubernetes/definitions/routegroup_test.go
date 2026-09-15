package definitions

import "testing"

func TestSkipperBackendNil(t *testing.T) {
	skipperBackend := `
{
  "name": "my-backend",
  "type": "network",
  "address": "http://example"
}
`

	var sb *SkipperBackend
	if err := sb.UnmarshalJSON([]byte(skipperBackend)); err != nil {
		t.Fatalf("Failed to get nil: %q", skipperBackend)
	}
}

func TestNewRouteGroupListWithSharedCache(t *testing.T) {
	list := NewRouteGroupListWithSharedCache([]*RouteGroupItem{
		{Spec: &RouteGroupSpec{}},
		nil,
		{Spec: &RouteGroupSpec{}},
		{Spec: nil},
	})

	f0, err := list.Items[0].Spec.ParseFilter(`status(200)`)
	if err != nil {
		t.Fatal(err)
	}
	f1, err := list.Items[2].Spec.ParseFilter(`status(200)`)
	if err != nil {
		t.Fatal(err)
	}
	if f0 != f1 {
		t.Error("expected the filter to be shared across RouteGroups")
	}

	p0, err := list.Items[0].Spec.ParsePredicate(`Method("GET")`)
	if err != nil {
		t.Fatal(err)
	}
	p1, err := list.Items[2].Spec.ParsePredicate(`Method("GET")`)
	if err != nil {
		t.Fatal(err)
	}
	if p0 != p1 {
		t.Error("expected the predicate to be shared across RouteGroups")
	}
}

func TestRouteGroupParseCacheIsPerRouteGroupWithoutSharing(t *testing.T) {
	a := &RouteGroupSpec{}
	b := &RouteGroupSpec{}

	fa, err := a.ParseFilter(`status(200)`)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := b.ParseFilter(`status(200)`)
	if err != nil {
		t.Fatal(err)
	}
	if fa == fb {
		t.Error("expected independent caches without shared cache")
	}
}
