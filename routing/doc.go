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

Request evaluation

1. The path in the http request is used to find one or more matching
route definitions in a lookup tree.

2. The rest of the request attributes is matched against the non-path
conditions of the routes found in the lookup tree, from the most to the
least strict one. The result is the first route whose every condition is
met.

(The regular expression conditions for the path, 'PathRegexp', are
applied only in the 2. step.)

Wildcards

Path matching supports two kinds of wildcards:

- simple wildcard: e.g. /some/:wildcard/path. Simple wildcards are
matching a single name in the request path.

- freeform wildcard: e.g. /some/path/*wildcard. Freeform wildcards are
matching any number of names at the end of the request path.

If a matched route contains wildcards in the path condition, the
wildcard keys mapped to the values will be returned with the route.

Data clients

Routing definitions are not directly passed to the routing instance, but
they are loaded from clients that implement the DataClient interface.
The router initially loads the complete set of the routes from each
client, merges the different sets based on the route id, and converts
them into their runtime representation, with initialized filters based
on the filter specifications in the filter registry.

During operation, the router regularly polls the data clients for
updates, and if an update is received, generates a new lookup tree. In
case of communication failure during polling, it reloads the whole set
of routes from the failing client.

For a full description of the route definitions, see the documentation
of the github.com/zalando/skipper/eskip package.
*/
package routing
