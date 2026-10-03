package hostpathmux

import (
	"testing"

	"github.com/zalando/skipper/pathmux"
)

type trueMatcher struct{}

func (m *trueMatcher) Match(v any) (bool, any) { return true, v }

func lookup(t *Tree, host, path string) any {
	lv, _, _ := t.Lookup(host, path, &trueMatcher{})
	return lv
}

func TestAddAndLookupExactHost(t *testing.T) {
	tree := New()
	if err := tree.Add("a.example.org", "/foo", "v1"); err != nil {
		t.Fatal(err)
	}

	if got := lookup(tree, "a.example.org", "/foo"); got != "v1" {
		t.Errorf("want v1, got %v", got)
	}
	if got := lookup(tree, "b.example.org", "/foo"); got != nil {
		t.Errorf("want nil for unknown host, got %v", got)
	}
}

func TestLookupWildcardFallback(t *testing.T) {
	tree := New()
	if err := tree.Add(WildcardHost, "/foo", "wildcard"); err != nil {
		t.Fatal(err)
	}

	if got := lookup(tree, "any.host", "/foo"); got != "wildcard" {
		t.Errorf("want wildcard, got %v", got)
	}
}

func TestHostBeforeWildcard(t *testing.T) {
	tree := New()
	if err := tree.Add("a.example.org", "/foo", "specific"); err != nil {
		t.Fatal(err)
	}
	if err := tree.Add(WildcardHost, "/foo", "wildcard"); err != nil {
		t.Fatal(err)
	}

	if got := lookup(tree, "a.example.org", "/foo"); got != "specific" {
		t.Errorf("want specific, got %v", got)
	}
	if got := lookup(tree, "other.host", "/foo"); got != "wildcard" {
		t.Errorf("want wildcard for other host, got %v", got)
	}
}

func TestLookupPathWildcards(t *testing.T) {
	tree := New()
	if err := tree.Add("a.example.org", "/api/:id", "api"); err != nil {
		t.Fatal(err)
	}

	lv, params, _ := tree.Lookup("a.example.org", "/api/42", &trueMatcher{})
	if lv == nil {
		t.Fatal("want match, got nil")
	}
	if len(params) == 0 || params[0] != "42" {
		t.Errorf("want params [42], got %v", params)
	}
}

func TestLookupNoMatch(t *testing.T) {
	tree := New()
	if err := tree.Add("a.example.org", "/foo", "v1"); err != nil {
		t.Fatal(err)
	}

	lv, params, extra := tree.Lookup("a.example.org", "/bar", &trueMatcher{})
	if lv != nil || params != nil || extra != nil {
		t.Errorf("want all nil for non-matching path, got %v %v %v", lv, params, extra)
	}
}

func TestLookupMatcherCanReject(t *testing.T) {
	tree := New()
	if err := tree.Add("a.example.org", "/foo", "v1"); err != nil {
		t.Fatal(err)
	}

	rejectMatcher := &rejectAll{}
	lv, _, _ := tree.Lookup("a.example.org", "/foo", rejectMatcher)
	if lv != nil {
		t.Errorf("want nil when matcher rejects, got %v", lv)
	}
}

type rejectAll struct{}

func (r *rejectAll) Match(v any) (bool, any) { return false, nil }

func TestMultipleHostsInTree(t *testing.T) {
	tree := New()
	_ = tree.Add("a.test", "/foo", "a")
	_ = tree.Add("b.test", "/foo", "b")
	_ = tree.Add("c.test", "/bar", "c")

	if got := lookup(tree, "a.test", "/foo"); got != "a" {
		t.Errorf("want a, got %v", got)
	}
	if got := lookup(tree, "b.test", "/foo"); got != "b" {
		t.Errorf("want b, got %v", got)
	}
	if got := lookup(tree, "c.test", "/bar"); got != "c" {
		t.Errorf("want c, got %v", got)
	}
	if got := lookup(tree, "a.test", "/bar"); got != nil {
		t.Errorf("want nil cross-host miss, got %v", got)
	}
}

func TestMatcherExtraValuePropagated(t *testing.T) {
	tree := New()
	_ = tree.Add("h.test", "/x", "stored")

	m := &extraMatcher{extra: "bonus"}
	_, _, extra := tree.Lookup("h.test", "/x", m)
	if extra != "bonus" {
		t.Errorf("want extra=bonus, got %v", extra)
	}
}

type extraMatcher struct{ extra any }

func (m *extraMatcher) Match(v any) (bool, any) { return true, m.extra }

// ensure Tree satisfies the pathmux.Matcher-consumer pattern at compile time
var _ pathmux.Matcher = (*trueMatcher)(nil)
