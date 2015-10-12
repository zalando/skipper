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
Package eskip implements a DSL for describing Skipper route expressions,
route definitions and complete routing tables.

Grammar Summary

A routing table is built up from 0 or more route definitions. The definitions
are separated by ';'. A route definition contains one route expression
prefixed by its ID and a ':'.

A routing table example:

    catalog: Path("/*category") -> "https://catalog.example.org";
    productPage: Path("/products/:id") -> "https://products.example.org";
    userAccount: Path("/user/:id/*userpage") -> "https://users.example.org";

    // 404
    notfound:
        Any() ->
        modPath(/.+/, "/notfound.html") -> static("/", "/var/www") ->
        <shunt>

A route expression always contains a matcher expression and a backend
expression, and it can contain optional filter expressions. The matcher
expression, each filter and the backend are separated by '->'. The filters
take place between the matcher and the backend.

A route expression example:

    Path("/api/*resource") && Header("Accept", "application/json") ->
    modPath("^/api", "") -> requestHeader("X-Type", "external") ->
    "https://api.example.org"

Matcher Expressions

The matcher expression contains one or more condition expressions. An
incoming request must fulfil each of them to match the route. Matcher
expressions are separated by '&&'.

A matcher expression example:

    Path("/api/*resource") && Header("Accept", "application/json")

The following matchers are recognized:

    Path("/some/path")

The path expression accepts a single parameter, that can be a fixed path like
"/some/path", or it can contain wildcards instead one or more names in the
path, e.g. "/some/:dir/:name", or it can end with a free wildcard like
"/some/path/*param", where the free wildcard can contain a sub-path with
multiple names. The parameters are available to the filters while processing
the matched requests.

    PathRegexp(/regular-expression/)

The regexp path expression accepts a regular expression as a single
parameter, that needs to be matched by the request path. The regular
expression can be enclosed by '/' or '"', and the escaping rules are applied
accordingly.

    Host(/host-regular-expression/)

The host expression accepts a regular expression as a single parameter that,
neds to be matched by the host information in the request.

    Method("HEAD")

The method expression is used to match the http request method.

    Header("Accept", "application/json")

The header expression is used to match the http headers in the request. It
accepts two parameters, the name of the header field and exact value to
match.

    HeaderRegexp("Accept", /\Wapplication\/json\W/)

The header regexp expression works similar to the header expression, but the
value to be matched is a regular expression.

    Any()

Catch all matcher.

Filters

Filters are used to augment the incoming requests and the outgoing responses,
or do other useful or fun stuff. Filters can have different numbers of
parameters depending on the implementation of the particular filter.

A filter example:

    responseHeader("max-age", "86400") -> static("/", "/var/www/public")

The default Skipper implementation provides the following builtin filters:

    requestHeader("header-name", "header-value")

    responseHeader("header-name", "header-value")

    healthcheck()

    modPath(/regular-expression/, "replacement")

    redirect(302, "https://ui.example.org")

    static("/images", "/var/www/images")

    stripQuery("true")

For the documentation about the builtin filters, please, refer to
the documentation of the github.com/zalando/skipper/filters package. Skipper
is designed to be extendable primarily by implementing custom filters, for
details about how to create custom filters, please, refer to the
documentation of the main Skipper package (github.com/zalando/skipper).

Backend

There are two types of backend: a network endpoint address or a shunt.

A network endpoint address:

    "http://internal.example.org:9090"

An endpoint address backend is surrounded by '"'. It contains the scheme
and the hostname of the endpoint, and optionally the port number that is
inferred from the scheme if not specified.

A shunt backend:

    <shunt>

The shunt backend means that the route will not forward the request to a
network endpoint, but the router will handle the request itself. By default,
the response is in this case 404 Not found, unless a filter in the route does
not change it.

Comments

Comments can be placed to document containing a routing table or just a
single route expression. The rule for comments is simple: everything is a
comment that starts with '//' and ends with a new-line character.

Example with comments:

    // forwards to the API endpoint
    route1: Path("/api") -> "https://api.example.org";
    route2: Any() -> <shunt> // everything else 404

Parsing filters

The eskip.ParseFilters method can be used to parse a chain of filters only,
without the matcher and backend part of the route expression.

Parsing

Parsing a routing table or a route expression happens with the eskip.Parse
function. In case of grammar error, it returns an error with approximate
position of the invalid syntax element, otherwise it returns a list of
structured in-memory route definitions.

The eskip parser does not validate the routes against semantic problems, e.g.
whether a matcher expression is valid, or a filter implementation is
available. This validation happens during processing the parsed definitions.

*/
package eskip
