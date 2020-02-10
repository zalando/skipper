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

OpenID Connect is an OAuth2.0 based authentication and authorization mechanism supported by
several providers. Skipper can act as a proxy for backend server which requires authenticated clients.
Skipper handles the authentication with the provider and upon sucessful completion of authentication
passes subsequent requests to the backend server.

Skipper's implementation of OpenID Connect Client works as follows:

1. Filter is initialized with the following parameters:
    1. Secrets file with keys used for encrypting the token in a cookie and also for generating shared secret.
    2. OpenID Connect Provider URL
    3. The Client ID
    4. The Client Secret
    5. The Callback URL for the client when a user successfully authenticates and is
        returned.
    6. The Scopes to be requested along with the `openid` scope
    7. The claims that should be present in the token or the fields need in the user
        information.
2. The user makes a request to a backend which is covered by an OpenID filter.
3. Skipper checks if a cookie is set with any previous successfully completed OpenID authentication.
4. If the cookie is valid then Skipper passes the request to the backend.
5. If the cookie is not valid then Skipper redirects the user to the OpenID provider with its Client ID and a callback URL.
6. When the user successfully completes authentication the provider redirects the user to the callback URL with a token.
7. Skipper receives this token and makes a backend channel call to get an ID token
    and other required information.
8. If all the user information/claims are present then it encrypts this and sets a cookie
    which is encrypted and redirects the user to the originally requested URL.
    
To use OpenID define a filter for a backend which needs to be covered by OpenID Connection authentication.

```
oauthOidcAllClaims("https://accounts.identity-provider.com", "some-client-id",
    "some-client-secret", "http://callback.com/auth/provider/callback", "scope1 scope2",
    "claim1 claim2") -> "https://internal.example.org";
```

Here `scope1 scope2` are the scopes that should be included which requesting authentication from the OpenID provider.
Any number of scopes can be specified here. The `openid` scope is added automatically by the filter. The other fields
which need to be specified are the URL of the provider which in the above example is
`https://accounts.identity-provider.com`. The client ID and the client secret. The callback URL which is specified
while generating the client id and client secret. Then the scopes and finally the claims which should be present along
with the return id token.

```
oauthOidcUserInfo("https://oidc-provider.example.com", "client_id", "client_secret",
    "http://target.example.com/subpath/callback", "email profile", 
    "name email picture") -> "https://internal.example.org";
```

This filter is similar but it verifies that the token has certain user information
information fields accesible with the token return by the provider. The fields can
be specified at the end like in the example above where the fields `name`, `email`
and `picture` are requested.

Upon sucessful authentication Skipper will start allowing the user requests through
to the backend. Along with the orginal request to the backend Skipper will include
information which it obtained from the provider. The information is in `JSON` format
with the header name `Skipper-Oidc-Info`. In the case of the claims container the
header value is in the format.

```json
{
    "oauth2token": "xxx",
    "claims": {
        "claim1": "val1",
        "claim2": "val2"
    },
    "subject": "subj"
}
```

In the case of a user info filter the payload is in the format:

```json
{
    "oauth2token": "xxx",
    "userInfo": {
        "sub": "sub",
        "profile": "prof",
        "email": "abc@example.com",
        "email_verified": "abc@example.com"
    },
    "subject": "subj"
}
```

Skipper encrypts the cookies and also generates a nonce during the OAuth2.0 flow
for which it needs a secret key. This key is in a file which can be rotated periodically
because it is reread by Skipper. The path to this file can be passed with the flag
`-oidc-secret-file` when Skipper is started.

### AuthZ and access control

Authorization validation and access control is available by means of a subsequent filter [oidcClaimsQuery](../reference/filters.md#oidcClaimsQuery). It inspects the ID token, which exists after a successful `oauthOidc*` filter step, and validates the defined query with the request path.

Given following example ID token:

```json
{
  "email": "someone@example.org",
  "groups": [
    "CD-xyz",
    "appX-Tester"
  ],
  "name": "Some One"
}
```

Access to path `/` would be granted to everyone in `example.org`, however path `/login` only to those being member of `group "appX-Tester"`:

```
oauthOidcAnyClaims(...) -> oidcClaimsQuery("/login:groups.#[==\"appX-Tester\"]", "/:@_:email%\"*@example.org\"")
```
