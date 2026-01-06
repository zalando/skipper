/*
Package routing implements matching of http requests to a continuously
updatable set of skipper routes.

# Request Evaluation

1. The path in the http request is used to find one or more matching
route definitions in a lookup tree.

2. The rest of the request attributes is matched against the non-path
conditions of the routes found in the lookup tree, from the most to the
least strict one. The result is the first route where every condition is
met.

(The regular expression conditions for the path, 'PathRegexp', are
applied only in step 2.)

The matching conditions and the built-in filters that use regular
expressions, use the go stdlib regexp, which uses re2:

https://github.com/google/re2/wiki/Syntax

# Matching Conditions

The following types of conditions are supported in the route definitions.

- Path: the route definitions may contain a single path condition,
optionally with wildcards, used for looking up routes in the lookup tree.

- PathSubtree: similar to Path, but used to match full subtrees including
the path of the definition.

- PathRegexp: regular expressions to match the path.

- Host: regular expressions that the host header in the request must
match.

- Method: the HTTP method that the request must match.

- Header: a header key and exact value that must be present in the
request. Note that Header("Key", "Value") is equivalent to
HeaderRegexp("Key", "^Value$").

- HeaderRegexp: a header key and a regular expression, where the key
must be present in the request and one of the associated values must
match the expression.

# Wildcards

Path matching supports two kinds of wildcards:

- simple wildcard: e.g. /some/:wildcard/path. Simple wildcards are
matching a single name in the request path.

- freeform wildcard: e.g. /some/path/*wildcard. Freeform wildcards are
matching any number of names at the end of the request path.

In case of PathSubtree, simple wildcards behave similar to Path, while
freeform wildcards only set the name of the path parameter containing
the path in the subtree. If no free wildcard is used in the PathSubtree
predicate, the name of this parameter will be "*". This makes the
PathSubtree("/foo") predicate equivalent to having routes with
Path("/foo"), Path("/foo/") and Path("/foo/**") predicates.

# Custom Predicates

It is possible to define custom route matching rules in the form of
custom predicates. Custom predicates need to implement the PredicateSpec
interface, that serves as a 'factory' and is used in the routing package
during constructing the routing tree. When a route containing a custom
predicate is matched based on the path tree, the predicate receives the
request object, and it returns true or false meaning that the request is
a match or not.

# Data Clients

Routing definitions are not directly passed to the routing instance, but
they are loaded from clients that implement the DataClient interface.
The router initially loads the complete set of the routes from each
client, merges the different sets based on the route id, and converts
them into their runtime representation, with initialized filters based
on the filter specifications in the filter registry.

During operation, the router regularly polls the data clients for
updates, and, if an update is received, generates a new lookup tree. In
case of communication failure during polling, it reloads the whole set
of routes from the failing client.

The active set of routes from the last successful update are used until
the next successful update happens.

Currently, the routes with the same id coming from different sources are
merged in a nondeterministic way, but this behavior may change in the
future.

For a full description of the route definitions, see the documentation
of the skipper/eskip package.
*/
package routing
