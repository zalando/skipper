// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
Package routing implements matching of http requests to a continuously
updatable set of skipper routes.


Request Evaluation

1. The path in the http request is used to find one or more matching
route definitions in a lookup tree.

2. The rest of the request attributes is matched against the non-path
conditions of the routes found in the lookup tree, from the most to the
least strict one. The result is the first route whose every condition is
met.

(The regular expression conditions for the path, 'PathRegexp', are
applied only in step 2.)


Matching Conditions

The following types of conditions are supported in the route definitions.

- Path: the route definitions may contain a single path condition,
optionally with wildcards, used to look up routes in the lookup tree.

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


Wildcards

Path matching supports two kinds of wildcards:

- simple wildcard: e.g. /some/:wildcard/path. Simple wildcards are
matching a single name in the request path.

- freeform wildcard: e.g. /some/path/*wildcard. Freeform wildcards are
matching any number of names at the end of the request path.


Data Clients

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

For a full description of the route definitions, see the documentation
of the skipper/eskip package.
*/
package routing
