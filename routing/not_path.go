package routing

import (
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/pathmux"
	"github.com/zalando/skipper/predicates"
	"net/http"
)

const magicRouteId = "42"

type notPathSpec struct{}

type notPathPredicate struct {
	path string
	tree *pathmux.Tree
}

func NewNotPath() PredicateSpec { return &notPathSpec{} }

func (n notPathSpec) Name() string {
	return predicates.NotPath
}

func (n notPathSpec) Create(args []interface{}) (Predicate, error) {
	if len(args) != 1 {
		return nil, predicates.ErrInvalidPredicateParameters
	}
	path, ok := args[0].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters

	}
	path, err := normalizePath(&Route{path: path})
	if err != nil {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	leaf, err := newLeaf(
		&Route{Route: eskip.Route{Id: magicRouteId}},
		nil)
	if err != nil {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	tree := &pathmux.Tree{}
	err = tree.Add(path, &pathMatcher{leaves: []*leafMatcher{leaf}})
	if err != nil {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	return &notPathPredicate{path: path, tree: tree}, nil
}

func (n notPathPredicate) Match(request *http.Request) bool {
	_, leafMatcher := matchPathTree(n.tree, request.RequestURI, &leafRequestMatcher{r: request})

	// No match is a successful predicate
	return leafMatcher == nil
}
