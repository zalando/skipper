/*
Package eskip implements an in-memory representation of Skipper routes
and a DSL for describing Skipper route expressions, route definitions
and complete routing tables.


Grammar Summary

A routing table is built up from 0 or more route definitions. The
definitions are separated by ';'. A route definition contains one route
expression prefixed by its id and a ':'.

A routing table example:

    catalog: Path("/*category") -> "https://catalog.example.org";
    productPage: Path("/products/:id") -> "https://products.example.org";
    userAccount: Path("/user/:id/*userpage") -> "https://users.example.org";

    // 404
    notfound: * ->
        modPath(/.+/, "/notfound.html") -> static("/", "/var/www") ->
        <shunt>

A route expression always contains a match expression and a backend
expression, and it can contain optional filter expressions. The match
expression, each filter and the backend are separated by '->'. The
filters take place between the matcher and the backend.

A route expression example:

    Path("/api/*resource") && Header("Accept", "application/json") ->
        modPath("^/api", "") -> requestHeader("X-Type", "external") ->
        "https://api.example.org"


Match Expressions - Predicates

A match expression contains one or more conditions. An incoming
request must fulfil each of them to match the route. The conditions are
separated by '&&'.

A match expression example:

    Path("/api/*resource") && Header("Accept", "application/json")

The following condition expressions are recognized:

    Path("/some/path")

The path condition accepts a single argument, that can be a fixed path
like "/some/path", or it can contain wildcards in place of one or more
names in the path, e.g. "/some/:dir/:name", or it can end with a free
wildcard like "/some/path/*param", where the free wildcard can contain a
sub-path with multiple names. Note, that this solution implicitly
supports the glob standard, e.g. "/some/path/**" will work as expected.
The arguments are available to the filters while processing the matched
requests.

    PathRegexp(/regular-expression/)

The regexp path condition accepts a regular expression as a single
argument that needs to be matched by the request path. The regular
expression can be surrounded by '/' or '"'.

    Host(/host-regular-expression/)

The host condition accepts a regular expression as a single argument
that needs to be matched by the host header in the request.

    Method("HEAD")

The method condition is used to match the http request method.

    Header("Accept", "application/json")

The header condition is used to match the http headers in the request.
It accepts two arguments, the name of the header field and the exact
header value to match.

    HeaderRegexp("Accept", /\Wapplication\/json\W/)

The header regexp condition works similar to the header expression, but
the value to be matched is a regular expression.

    *

Catch all condition.

    Any()

Former, deprecated form of the catch all condition.


Custom Predicates

Eskip supports custom route matching predicates, that can be implemented
in extensions. The notation of custom predicates is the same as of the
built-in route matching expressions:

    Foo(3.14, "bar")

During parsing, custom predicates may define any arbitrary list of
arguments of types number, string or regular expression, and it is the
responsibility of the implementation to validate them.

(See the documentation of the routing package.)


Filters

Filters are used to augment the incoming requests and the outgoing
responses, or do other useful or fun stuff. Filters can have different
numbers of arguments depending on the implementation of the particular
filter. The arguments can be of type string ("a string"), number
(3.1415) or regular expression (/[.]html$/ or "[.]html$").

A filter example:

    setResponseHeader("max-age", "86400") -> static("/", "/var/www/public")

The default Skipper implementation provides the following built-in
filters:

    setRequestHeader("header-name", "header-value")

    setResponseHeader("header-name", "header-value")

    appendRequestHeader("header-name", "header-value")

    appendResponseHeader("header-name", "header-value")

    dropRequestHeader("header-name")

    dropResponseHeader("header-name")

    modPath(/regular-expression/, "replacement")

    redirectTo(302, "https://ui.example.org")

    flowId("reuse", 64)

    healthcheck()

    static("/images", "/var/www/images")

    stripQuery("true")

    preserveHost()

	status(418)

For details about the built-in filters, please, refer to the
documentation of the skipper/filters package. Skipper is designed to be
extendable primarily by implementing custom filters, for details about
how to create custom filters, please, refer to the documentation of the
root skipper package.


Backend

There are two types of backends: a network endpoint address or a shunt.

A network endpoint address example:

    "http://internal.example.org:9090"

An endpoint address backend is surrounded by '"'. It contains the scheme
and the hostname of the endpoint, and optionally the port number that is
inferred from the scheme if not specified.

A shunt backend:

    <shunt>

The shunt backend means that the route will not forward the request to a
network endpoint, but the router will handle the request itself. By
default, the response is in this case 404 Not found, unless a filter in
the route does not change it.


Comments

An eskip document can contain comments. The rule for comments is simple:
everything is a comment that starts with '//' and ends with a new-line
character.

Example with comments:

    // forwards to the API endpoint
    route1: Path("/api") -> "https://api.example.org";
    route2: * -> <shunt> // everything else 404


Regular expressions

The matching conditions and the built-in filters that use regular
expressions, use the go stdlib regexp, which uses re2:

https://github.com/google/re2/wiki/Syntax


Parsing Filters

The eskip.ParseFilters method can be used to parse a chain of filters,
without the matcher and backend part of a full route expression.


Parsing

Parsing a routing table or a route expression happens with the
eskip.Parse function. In case of grammar error, it returns an error with
the approximate position of the invalid syntax element, otherwise it
returns a list of structured, in-memory route definitions.

The eskip parser does not validate the routes against all semantic rules,
e.g., whether a filter or a custom predicate implementation is available.
This validation happens during processing the parsed definitions.


Serializing

Serializing a single route happens by calling its String method.
Serializing a complete routing table happens by calling the
eskip.String method.
*/
package eskip
