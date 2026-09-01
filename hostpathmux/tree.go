package hostpathmux

import "github.com/zalando/skipper/pathmux"

// WildcardHost is the key used for routes that have no exact-host constraint.
// Routes added under this key are tried as a fallback when no per-host subtree
// produces a match.
const WildcardHost = "*"

// Tree is a two-level routing trie. The outer level is keyed by exact hostname
// strings; the inner level is a pathmux.Tree keyed by path.
type Tree struct {
	hosts map[string]*pathmux.Tree
}

// New returns an empty Tree.
func New() *Tree {
	return &Tree{hosts: make(map[string]*pathmux.Tree)}
}

// Add registers value at the given host+path combination.
// host must be a literal hostname (e.g. from HostAny) or WildcardHost.
// path follows the same wildcard syntax as pathmux.Tree.Add.
func (t *Tree) Add(host, path string, value any) error {
	pt, ok := t.hosts[host]
	if !ok {
		pt = &pathmux.Tree{}
		t.hosts[host] = pt
	}
	return pt.Add(path, value)
}

// Lookup finds the best match for the given host and path.
// It first queries the per-host subtree for host; on failure it falls back to
// the WildcardHost subtree. The Matcher is forwarded to pathmux.Tree.LookupMatcher
// for leaf selection.
func (t *Tree) Lookup(host, path string, m pathmux.Matcher) (any, []string, any) {
	if pt, ok := t.hosts[host]; ok {
		lv, params, extra := pt.LookupMatcher(path, m)
		if lv != nil {
			return lv, params, extra
		}
	}
	if pt, ok := t.hosts[WildcardHost]; ok {
		lv, params, extra := pt.LookupMatcher(path, m)
		if lv != nil {
			return lv, params, extra
		}
	}
	return nil, nil, nil
}
