// Package hostpathmux implements a two-level routing trie for HTTP routing.
//
// The outer level is keyed by exact hostname strings; the inner level is a
// pathmux.Tree keyed by request path. Routes indexed under WildcardHost ("*")
// are tried as a fallback when no per-host subtree produces a match.
//
// This is used by the routing package when UseHostTree is enabled to reduce
// the candidate set for leaf evaluation in workloads dominated by host-specific
// routes using the HostAny predicate.
package hostpathmux
