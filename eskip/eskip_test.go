package eskip

import "testing"

func TestParsePathMatcher(t *testing.T) {
	r, err := Parse(`Path("/some/path") -> "https://www.example.org"`)
	if err != nil {
		t.Error(err)
	}

	if len(r) != 1 || r[0].Path != "/some/path" {
		t.Error("failed to parse path matcher")
	}
}

func TestParseMethodMatcher(t *testing.T) {
	r, err := Parse(`Method("HEAD") -> "https://www.example.org"`)
	if err != nil {
		t.Error(err)
	}

	if len(r) != 1 || r[0].Method != "HEAD" {
		t.Error("failed to parse method matcher")
	}
}

func TestParseHostRegexpsMatcher(t *testing.T) {
	r, err := Parse(`Host(/^www[.]/) && Host(/[.]org$/) -> "https://www.example.org"`)
	if err != nil {
		t.Error(err)
	}

	if len(r) != 1 || len(r[0].HostRegexps) != 2 ||
		r[0].HostRegexps[0] != "^www[.]" || r[0].HostRegexps[1] != "[.]org$" {
		t.Error("failed to parse host regexp matchers")
	}
}

func TestParsePathRegexpsMatcher(t *testing.T) {
	r, err := Parse(`PathRegexp("^/some") && PathRegexp(/\/\w+Id$/) -> "https://www.example.org"`)
	if err != nil {
		t.Error(err)
	}

	if len(r) != 1 || len(r[0].PathRegexps) != 2 ||
		r[0].PathRegexps[0] != "^/some" || r[0].PathRegexps[1] != "/\\w+Id$" {
		t.Error("failed to parse path regexp matchers")
	}
}

func TestParseHeaderRegexps(t *testing.T) {
	r, err := Parse(`
		HeaderRegexp("Header-0", "value-0") &&
		HeaderRegexp("Header-0", "value-1") &&
		HeaderRegexp("Header-1", "value-2") &&
		HeaderRegexp("Header-1", "value-3") ->
		"https://www.example.org"`)
	if err != nil {
		t.Error(err)
	}

	if len(r) != 1 || len(r[0].HeaderRegexps) != 2 ||
		len(r[0].HeaderRegexps["Header-0"]) != 2 ||
		r[0].HeaderRegexps["Header-0"][0] != "value-0" ||
		r[0].HeaderRegexps["Header-0"][1] != "value-1" ||
		r[0].HeaderRegexps["Header-1"][0] != "value-2" ||
		r[0].HeaderRegexps["Header-1"][1] != "value-3" {
		t.Error("failed to parse header regexps")
	}
}

func TestParseHeaders(t *testing.T) {
	r, err := Parse(`
		Header("Header-0", "value-0") &&
		Header("Header-1", "value-1") ->
		"https://www.example.org"`)
	if err != nil {
		t.Error(err)
	}

	if len(r) != 1 || len(r[0].Headers) != 2 ||
		r[0].Headers["Header-0"] != "value-0" ||
		r[0].Headers["Header-1"] != "value-1" {
		t.Error("failed to parse headers")
	}
}

func TestParseFiltersEmpty(t *testing.T) {
	fs, err := ParseFilters(" \t")
	if err != nil || len(fs) != 0 {
		t.Error("failed to parse empty filter expression")
	}
}

func TestParseFiltersError(t *testing.T) {
	_, err := ParseFilters("trallala")
	if err == nil {
		t.Error("failed to fail")
	}
}

func TestParseFilters(t *testing.T) {
	fs, err := ParseFilters(`filter1(3.14) -> filter2("key", 42)`)
	if err != nil || len(fs) != 2 ||
		fs[0].Name != "filter1" || len(fs[0].Args) != 1 ||
		fs[0].Args[0] != float64(3.14) ||
		fs[1].Name != "filter2" || len(fs[1].Args) != 2 ||
		fs[1].Args[0] != "key" || fs[1].Args[1] != float64(42) {
		t.Error("failed to parse filters")
	}
}
