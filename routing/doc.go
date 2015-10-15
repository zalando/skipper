// Package requestmatch implements matching http requests to associated values.
//
// Matching is based on the attributes of http requests, where a request matches
// a definition if it fulfills all condition in it. The evaluation happens in the
// following order:
//
// 1. The request path is used to find leaf definitions in a lookup tree. If no
// path match was found, the leaf definitions in the root are taken that don't
// have a condition for path matching.
//
// 2. If any leaf definitions were found, they are evaluated against the request
// and the associated value of the first matching definition is returned. The order
// of the evaluation happens from the strictest definition to the least strict
// definition, where strictness is proportional to the number of non-empty
// conditions in the definition.
//
// Path matching supports two kind of wildcards:
//
// - a simple wildcard matches a single tag in a path. E.g: /users/:name/roles
// will be matched by /users/jdoe/roles, and the value of the parameter 'name' will
// be 'jdoe'
//
// - a freeform wildcard matches the last segment of a path, with any number of
// tags in it. E.g: /assets/*assetpath will be matched by /assets/images/logo.png,
// and the value of the parameter 'assetpath' will be '/images/logo.png'.

/*
mathcing http requests to skipper routes

using an internal lookup tree

matches if all conditions fulfilled in the route

evaluation order:

1. path in the lookup tree for leaf definitions, if no match leaf definitions in the
root. root leaf that have no path condition (no regexp)

2. in the leaf matching based on the rest of the conditions, from the most strict to the
least strict

path matching supports two kinds of wildcards

- simple wildcard matching a single name in the path

- freeform wildcard at the end of the path matching multiple names

wildcards in the response

routing definitions from data clients, merged, poll timeout

route definitions converted to routes with real filter instances using the registry

tail slash option
*/
package routing
