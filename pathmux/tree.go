/*
Package pathmux implements a tree lookup for values associated to
paths.

This package is a fork of https://github.com/dimfeld/httptreemux.
*/
package pathmux

import (
	"fmt"
	"net/url"
	"strings"
)

// Matcher objects, when using the LookupMatcher function, can be used for additional checks and to override the
// default result in case of path matches. The argument passed to the Match function is the original value
// passed to the Tree.Add function.
type Matcher interface {

	// Match should return true and the object to be returned by the lookup, when the argument value fulfils the
	// conditions defined by the custom logic in the matcher itself. If it returns false, it instructs the
	// lookup to continue with backtracking from the current tree position.
	Match(value any) (bool, any)
}

type trueMatcher struct{}

func (m *trueMatcher) Match(value any) (bool, any) {
	return true, value
}

var tm *trueMatcher

func init() {
	tm = &trueMatcher{}
}

type node struct {
	path string

	priority int

	// The list of static children to check.
	staticIndices []byte
	staticChild   []*node

	// If none of the above match, check the wildcard children
	wildcardChild *node

	// If none of the above match, then we use the catch-all, if applicable.
	catchAllChild *node

	isCatchAll bool

	leafValue any
}

// Tree structure to store values associated to paths.
type Tree node

func (n *node) sortStaticChild(i int) {
	for i > 0 && n.staticChild[i].priority > n.staticChild[i-1].priority {
		n.staticChild[i], n.staticChild[i-1] = n.staticChild[i-1], n.staticChild[i]
		n.staticIndices[i], n.staticIndices[i-1] = n.staticIndices[i-1], n.staticIndices[i]
		i -= 1
	}
}

func (n *node) addPath(path string) (*node, error) {
	leaf := len(path) == 0
	if leaf {
		return n, nil
	}

	c := path[0]
	nextSlash := strings.Index(path, "/")
	var thisToken string
	var tokenEnd int

	if c == '/' {
		thisToken = "/"
		tokenEnd = 1
	} else if nextSlash == -1 {
		thisToken = path
		tokenEnd = len(path)
	} else {
		thisToken = path[0:nextSlash]
		tokenEnd = nextSlash
	}
	remainingPath := path[tokenEnd:]

	if c == '*' {
		// Token starts with a *, so it's a catch-all
		thisToken = thisToken[1:]
		if n.catchAllChild == nil {
			n.catchAllChild = &node{path: thisToken, isCatchAll: true}
		}

		if path[1:] != n.catchAllChild.path {
			return nil, fmt.Errorf(
				"catch-all name in %s doesn't match %s",
				path, n.catchAllChild.path)
		}

		if nextSlash != -1 {
			return nil, fmt.Errorf("/ after catch-all found in %s", path)
		}

		return n.catchAllChild, nil
	} else if c == ':' {
		// Token starts with a :
		if n.wildcardChild == nil {
			n.wildcardChild = &node{path: "wildcard"}
		}

		return n.wildcardChild.addPath(remainingPath)

	} else {
		if strings.ContainsAny(thisToken, ":*") {
			return nil, fmt.Errorf("* or : in middle of path component %s", path)
		}

		// Do we have an existing node that starts with the same byte?
		for i, index := range n.staticIndices {
			if c == index {
				// Yes. Split it based on the common prefix of the existing
				// node and the new one.
				child, prefixSplit := n.splitCommonPrefix(i, thisToken)
				child.priority++
				n.sortStaticChild(i)
				return child.addPath(path[prefixSplit:])
			}
		}

		// No existing node starting with this byte, so create it.
		child := &node{path: thisToken}

		if n.staticIndices == nil {
			n.staticIndices = []byte{c}
			n.staticChild = []*node{child}
		} else {
			n.staticIndices = append(n.staticIndices, c)
			n.staticChild = append(n.staticChild, child)
		}
		return child.addPath(remainingPath)
	}
}

func (n *node) splitCommonPrefix(existingNodeIndex int, path string) (*node, int) {
	childNode := n.staticChild[existingNodeIndex]

	if strings.HasPrefix(path, childNode.path) {
		// No split needs to be done. Rather, the new path shares the entire
		// prefix with the existing node, so the new node is just a child of
		// the existing one. Or the new path is the same as the existing path,
		// which means that we just move on to the next token. Either way,
		// this return accomplishes that
		return childNode, len(childNode.path)
	}

	// Find the length of the common prefix of the child node and the new path.
	i := commonPrefixLen(childNode.path, path)

	commonPrefix := path[0:i]
	childNode.path = childNode.path[i:]

	// Create a new intermediary node in the place of the existing node, with
	// the existing node as a child.
	newNode := &node{
		path:     commonPrefix,
		priority: childNode.priority,
		// Index is the first byte of the non-common part of the path.
		staticIndices: []byte{childNode.path[0]},
		staticChild:   []*node{childNode},
	}
	n.staticChild[existingNodeIndex] = newNode

	return newNode, i
}

func commonPrefixLen(x, y string) int {
	n := 0
	for n < len(x) && n < len(y) && x[n] == y[n] {
		n++
	}
	return n
}

func (n *node) search(path string, m Matcher) (found *node, params []string, value any) {
	pathLen := len(path)
	if pathLen == 0 {
		if n.leafValue == nil {
			return nil, nil, nil
		}

		var match bool
		match, value = m.Match(n.leafValue)

		if !match {
			return nil, nil, nil
		}

		return n, nil, value
	}

	// First see if this matches a static token.
	firstChar := path[0]
	for i, staticIndex := range n.staticIndices {
		if staticIndex == firstChar {
			child := n.staticChild[i]
			childPathLen := len(child.path)
			if pathLen >= childPathLen && child.path == path[:childPathLen] {
				nextPath := path[childPathLen:]
				found, params, value = child.search(nextPath, m)
			}
			break
		}
	}

	if found != nil {
		return
	}

	if n.wildcardChild != nil {
		// Didn't find a static token, so check for a wildcard.
		nextSlash := 0
		for nextSlash < pathLen && path[nextSlash] != '/' {
			nextSlash++
		}

		thisToken := path[0:nextSlash]
		nextToken := path[nextSlash:]

		if len(thisToken) > 0 { // Don't match on empty tokens.
			found, params, value = n.wildcardChild.search(nextToken, m)
			if found != nil {
				unescaped, err := url.QueryUnescape(thisToken)
				if err != nil {
					unescaped = thisToken
				}

				if params == nil {
					params = []string{unescaped}
				} else {
					params = append(params, unescaped)
				}

				return
			}
		}
	}

	catchAllChild := n.catchAllChild
	if catchAllChild != nil {
		// Hit the catchall, so just assign the whole remaining path.
		unescaped, err := url.QueryUnescape(path)
		if err != nil {
			unescaped = path
		}

		var match bool
		match, value = m.Match(catchAllChild.leafValue)

		if !match {
			return nil, nil, nil
		}

		return catchAllChild, []string{unescaped}, value
	}

	return nil, nil, nil
}

// Add a value to the tree associated with a path. Paths may contain
// wildcards. Wildcards can be of two types:
//
// - simple wildcard: e.g. /some/:wildcard/path, where a wildcard is
// matched to a single name in the path.
//
// - free wildcard: e.g. /some/path/*wildcard, where a wildcard at the
// end of a path matches anything.
func (t *Tree) Add(path string, value any) error {
	n, err := (*node)(t).addPath(path[1:])
	if err != nil {
		return err
	}

	n.leafValue = value
	return nil
}

// Lookup tries to find a value in the tree associated to a path. If the found path definition contains
// wildcards, the values of the wildcards are returned in the second argument.
func (t *Tree) Lookup(path string) (any, []string) {
	node, params, _ := t.LookupMatcher(path, tm)
	return node, params
}

// LookupMatcher tries to find value in the tree associated to a path. If the found path definition contains
// wildcards, the values of the wildcards are returned in the second argument. When a value is found,
// the matcher is called to check if the value meets the conditions implemented by the custom matcher. If it
// returns true, then the lookup is done and the additional return value from the matcher is returned as the
// lookup result. If it returns false, the lookup continues with backtracking from the current tree position.
func (t *Tree) LookupMatcher(path string, m Matcher) (any, []string, any) {
	if path == "" {
		path = "/"
	}

	node, params, value := (*node)(t).search(path[1:], m)
	if node == nil {
		return nil, nil, nil
	}

	return node.leafValue, params, value
}
