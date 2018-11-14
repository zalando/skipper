## Basic auth

Basic Auth is defined in [RFC7617](https://tools.ietf.org/html/rfc7617).

Install htpasswd command line tool, we assume Debian based
system. Please refer the documentation of your Operating System or
package management vendor how to install `htpasswd`:

```
apt-get install apache2-utils
```

Create a htpasswd file `foo.passwd` and use `captain` with password `apassword`:

```
htpasswd -bcB foo.passwd captain apassword
```

Start skipper with a `basicAuth` filter referencing the just created
htpasswd file:

```
./bin/skipper -address :8080 -inline-routes 'r: * -> basicAuth("foo.passwd") -> status(200) -> <shunt>'
```

A client request without login credentials or wrong credentials:

```
% curl localhost:8080/ -v
*   Trying ::1...
* Connected to localhost (::1) port 8080 (#0)
> GET / HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.49.0
> Accept: */*
>
< HTTP/1.1 401 Unauthorized
< Server: Skipper
< Www-Authenticate: Basic realm="Basic Realm"
< Date: Thu, 01 Nov 2018 21:27:18 GMT
< Content-Length: 0
<
* Connection #0 to host localhost left intact

```

A client request with the correct credentials:

```
% curl captain:apassword@localhost:8080/ -v
*   Trying ::1...
* Connected to localhost (::1) port 8080 (#0)
* Server auth using Basic with user 'captain'
> GET / HTTP/1.1
> Host: localhost:8080
> Authorization: Basic Y2FwdGFpbjphcGFzc3dvcmQ=
> User-Agent: curl/7.49.0
> Accept: */*
>
< HTTP/1.1 200 OK
< Server: Skipper
< Date: Thu, 01 Nov 2018 21:29:21 GMT
< Content-Length: 0
<
* Connection #0 to host localhost left intact
```

## Token service-to-service

Service to service authentication and authorization is often done by
using the HTTP Authorization header with the content prefix "Bearer ",
for example "Authorization: Bearer mytoken".

Supported token formats

- [OAuth2 access tokens](https://tools.ietf.org/html/rfc6750)
- [JWT](https://tools.ietf.org/html/rfc7519)

### Tokeninfo

Tokeninfo is a common, but not specified protocol, only supporting
Bearer tokens in the Authorization header.

In most cases you would have to have your own OAuth2 token
infrastructure, that can return JWT or OAuth2 access tokens to authenticated parties
and validate tokens with their custom tokeninfo endpoint. In case of
JWT the access token is signed and can be validated without a central
tokeninfo endpoint.

Example route:


```
all: Path("/")
     -> oauthTokeninfoAnyScope("read-X", "readwrite-X")
     -> "http://localhost:9090/"
```

The access token should be passed from the client as Bearer token in
the Authorization header. Skipper will send this token unchanged as
Bearer token in the Authorization header to the Tokeninfo endpoint.
The request flow with a Tokeninfo setup is shown in the following
picture:

![Skipper with Tokeninfo](../img/svc-to-svc-tokeninfo.svg)

### Tokenintrospection RFC7662

Tokenintrospection service to service authentication and authorization
is specified by [RFC7662](https://tools.ietf.org/html/rfc7662).
Skipper uses [RFC Draft for discovering token infrastructure
configuration](https://tools.ietf.org/html/draft-ietf-oauth-discovery-06),
to find the `introspection_endpoint`.

Example route:


```
all: *
        -> oauthTokenintrospectionAnyKV("https://identity.example.com/managed-id", "jdoe")
        -> "http://localhost:9090/";
```

The access token should be passed from the client as Bearer token in
the Authorization header. Skipper will send this token as
defined in [RFC7662](https://tools.ietf.org/html/rfc7662#section-2.1)
in a POST request "application/x-www-form-urlencoded" as value for key
`token` to the Tokenintrospection endpoint.
The request flow with Tokenintrospection setup is shown in the
following picture:

![Skipper with Tokenintrospection](../img/svc-to-svc-tokenintrospection.svg)

## OpenID Connect

### UI authcode flow

The pull request implements the authcode flow, which can be used for
UI authentication and authorization.

[https://github.com/zalando/skipper/pull/743](https://github.com/zalando/skipper/pull/743)
