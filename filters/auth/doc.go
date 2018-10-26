/*
Package auth provides authentication related filters.

Basic - Check Basic Authentication

The filter accepts two parameters, the first mandatory one is the path to the htpasswd file usually used with
Apache or nginx. The second one is the optional realm name that will be displayed in the browser. Each incoming
request will be validated against the password file, for more information which formats are currently supported
check "https://github.com/abbot/go-http-auth". Assuming that the MD5 version will be used, new entries can be generated like

	htpasswd -nbm myName myPassword

Embedding the filter in routes:

	generic: * -> basicAuth("/path/to/htpasswd") -> "https://internal.example.org";

	myRealm: Host("my.example.org")
	  -> basicAuth("/path/to/htpasswd", "My Website")
	  -> "https://my-internal.example.org";

OAuth2 - Check Bearer Tokens

The auth filter takes the incoming request, and tries to extract the
Bearer token from the Authorization header. Then it validates against
a configured service. Depending on the settings, it also can check if
the owner of the token belongs to a specific OAuth2 realm, and it can
check if it has at least one of the predefined scopes. If any of the
expectations are not met, it doesn't forward the request to the target
endpoint, but returns with status 401.

OAuth2 - Provider Configuration - Tokeninfo

To enable OAuth2 tokeninfo filters you have to set the CLI argument
-oauth2-tokeninfo-url=<OAuthTokeninfoURL>. Scopes and key value pairs
depend on the OAuth2 tokeninfo provider. AccessTokens has to be
accepted by your OAuth2 provider's TokeninfoURL. Filter names starting
with oauthTokeninfo will work on the returned data from
TokeninfoURL. The request from skipper to TokeninfoURL will use the
`Authorization: Bearer <access_token>` Header to do the request.

Additionally, you can also pass CLI argument
-oauth2-tokeninfo-timeout=<OAuthTokeninfoTimeout> to control the
default timeout duration for OAuth validation request. The default
tokeninfo timeout is 2s.

Example json output of the tokeninfo response could be:

    {
      "access_token": "<mytoken>",
      "client_id": "ztoken",
      "cn": "Jane Doe",
      "expires_in": "300",
      "grant_type": "password",
      "realm": "/employees",
      "scope": [
        "uid",
        "foo-r",
        "bar-w",
        "qux-rw"
      ],
      "token_type": "Bearer",
      "uid": "jdoe"
    }

OAuth2 - oauthTokeninfoAnyScope filter

The filter oauthTokeninfoAnyScope allows access if one of the scopes
is satisfied by the request.

    a: Path("/a") -> oauthTokeninfoAnyScope("uid") -> "https://internal.example.org/";
    b: Path("/b") -> oauthTokeninfoAnyScope("uid", "bar") -> "https://internal.example.org/";

OAuth - oauthTokeninfoAllScope() filter

The filter oauthTokeninfoAllScope allows access if all of the scopes
are satisfied by the request:

    a: Path("/a") -> oauthTokeninfoAllScope("uid") -> "https://internal.example.org/";
    b: Path("/b") -> oauthTokeninfoAllScope("uid", "bar") -> "https://internal.example.org/";

OAuth - oauthTokeninfoAnyKV() filter

The filter oauthTokeninfoAnyKV allows access if the token information
returned by OAuthTokeninfoURL has the given key and the given
value.

The following route has a filter definition, that one of the keys
"uid" or "foo" has the value "jdoe" or "bar". Additionally the second
will check if there is a "realm" "/employees":

    a: Path("/") -> oauthTokeninfoAnyKV("uid", "jdoe", "foo", "bar") -> "https://internal.example.org/";
    b: Path("/") -> oauthTokeninfoAnyKV("realm","/employees", "uid", "jdoe", "foo", "bar") -> "https://internal.example.org/";

Example json output of this tokeninfo response:

    {
      "access_token": "<mytoken>",
      "client_id": "ztoken",
      "cn": "Jane Doe",
      "expires_in": "300",
      "grant_type": "password",
      "realm": "/employees",
      "scope": [
        "uid",
        "foo-r",
        "bar-w",
        "qux-rw"
      ],
      "token_type": "Bearer",
      "uid": "jdoe"
    }

OAuth - oauthTokeninfoAllKV() filter

The filter oauthTokeninfoAllKV allows access if the token information
returned by OAuthTokeninfoURL has the given key and the given value.

The following route has a filter definition, that will check if all of
the key value pairs match. Here "uid" has to have the value "jdoe" and
"foo" has to have the value "bar". Additionally the second will
check if there is a "realm" "/employees":

    a: Path("/") -> oauthTokeninfoAllKV("uid", "jdoe", "foo", "bar") -> "https://internal.example.org/";
    b: Path("/") -> oauthTokeninfoAllKV("realm", "/employees", "uid", "jdoe", "foo", "bar") -> "https://internal.example.org/";

Example json output of this information response:

    {
      "access_token": "<mytoken>",
      "client_id": "ztoken",
      "cn": "John Doe",
      "expires_in": "300",
      "grant_type": "password",
      "foo": "bar",
      "realm": "/employees",
      "scope": [
        "uid",
        "foo-r",
        "bar-w",
        "qux-rw"
      ],
      "token_type": "Bearer",
      "uid": "jdoe"
    }


In case you are using any of the above 4 filters in your custom build,
you can call the `Close()` method to close the `quit` channel and
free up goroutines, to avoid goroutine leak

OAuth2 - Provider Configuration - Tokenintrospection

Provider configuration is dynamically done by
https://tools.ietf.org/html/draft-ietf-oauth-discovery-06#section-5,
which means a GET /.well-known/openid-configuration to the issuer URL.
Skipper will use the `introspection_endpoint` to configure the target
to query for information and use `claims_supported` to validate valid
filter configurations.

Example response from the openid-configuration endpoint:

    {
      "issuer"                : "https://issuer.example.com",
      "token_endpoint"        : "https://issuer.example.com/token",
      "introspection_endpoint": "https://issuer.example.com/token/introspect",
      "revocation_endpoint"   : "https://issuer.example.com/token/revoke",
      "authorization_endpoint": "https://issuer.example.com/login",
      "userinfo_endpoint"     : "https://issuer.example.com/userinfo",
      "jwks_uri"              : "https://issuer.example.com/token/certs",
      "response_types_supported": [
        "code",
        "token",
        "id_token",
        "code token",
        "code id_token",
        "token id_token",
        "code token id_token",
        "none"
      ],
      "subject_types_supported": [
        "public"
      ],
      "id_token_signing_alg_values_supported": [
        "RS256"
      ],
      "scopes_supported": [
        "openid",
        "email",
        "profile"
      ],
      "token_endpoint_auth_methods_supported": [
        "client_secret_post",
        "client_secret_basic"
      ],
      "claims_supported": [
        "aud",
        "email",
        "email_verified",
        "exp",
        "family_name",
        "given_name",
        "iat",
        "iss",
        "locale",
        "name",
        "picture",
        "sub"
      ],
      "code_challenge_methods_supported": [
        "plain",
        "S256"
      ]
    }

Additionally, you can also pass CLI argument
-oauth2-tokenintrospect-timeout=<OAuthTokenintrospectTimeout> to control the
default timeout duration for OAuth validation request. The default
tokenintrospect timeout is 2s.

All oauthTokenintrospection* filters will work on the tokenintrospect response.

Example json output of the tokenintrospect response could be:


    {
      "access_token": "<mytoken>",
      "client_id": "ztoken",
      "name": "Jane Doe",
      "expires_in": "300",
      "grant_type": "password",
      "active": true,
      "sub": "a-sub",
      "iss": "https://issuer.example.com"
      "realm": "/employees",
      "claims": {
        "uid": "jdoe",
        "email": "jdoe@example.com"
      },
      "scope": [
        "email",
        "foo-r",
      ],
      "token_type": "Bearer",
    }


OAuth2 - oauthTokenintrospectionAnyClaims filter

The filter oauthTokenintrospectionAnyClaims can be configured with
claims validated from the openid-configuration `claims_supported` and
will use the `introspection_endpoint` endpoint to query for
the token information.

The filter oauthTokenintrospectionAnyClaims allows access if the token
information has at least one of the claims in the token as configured
in the filter.

The following route has a filter definition, that will check if there
is one of the following claims in the token: "uid" or "email":

    a: Path("/") -> oauthTokenintrospectionAnyClaims("https://issuer.example.com", "uid", "email") -> "https://internal.example.org/";

OAuth2 - oauthTokenintrospectionAllClaims filter

The filter oauthTokenintrospectionAllClaims can be configured with
claims validated from the openid-configuration `claims_supported` and
will use the `introspection_endpoint` endpoint to query for
the token information.

The filter oauthTokenintrospectionAllClaims allows access if the token
information has at least one of the claims in the token as configured
in the filter.

The following route has a filter definition, that will check if there
all of the following claims in the token: "uid" and "email":

    a: Path("/") -> oauthTokenintrospectionAllClaims("https://issuer.example.com", "uid", "email") -> "https://internal.example.org/";

OAuth2 - oauthTokenintrospectionAnyKV filter

The filter oauthTokenintrospectionAnyKV will use the
`introspection_endpoint` endpoint from the openid-configuration to
query for the token information.

The filter oauthTokenintrospectionAnyKV allows access if the token
information has at least one of the key-value pairs in the token as
configured in the filter.

The following route has a filter definition, that will check if there
one of the following key-value pairs in the token: "uid=jdoe" or
"iss=https://issuer.example.com":

    a: Path("/") -> oauthTokenintrospectionAnyKV("https://issuer.example.com", "uid", "jdoe", "iss", "https://issuer.example.com") -> "https://internal.example.org/";

OAuth2 - oauthTokenintrospectionAllKV filter

The filter oauthTokenintrospectionAllKV will use the
`introspection_endpoint` endpoint from the openid-configuration to
query for the token information.

The filter oauthTokenintrospectionAnyKV allows access if the token
information has all of the key-value pairs in the token as
configured in the filter.

The following route has a filter definition, that will check if there
are all of the following key-value pairs in the token: "uid=jdoe" or
"iss=https://issuer.example.com":

    a: Path("/") -> oauthTokenintrospectionAllKV("https://issuer.example.com", "uid", "jdoe", "iss", "https://issuer.example.com") -> "https://internal.example.org/";

OpenID - oauthOidcUserInfo filter

The filter oauthOidcUserInfo is a filter for OAuth Implicit Flow authentication of users. It verifies that the token
provided by the user upon authentication contains all the fields specified in the filter.

	a: Path("/") -> oauthOidcUserInfo("https://accounts.identity-provider.com", "some-client-id", "some-client-secret", "http://callback.com/auth/provider/callback", "field1", "field2") -> "https://internal.example.org";

OpenID - oauthOidcAnyClaims filter

The filter oauthOidcAnyClaims is a filter for OAuth Implicit Flow authentication scheme for users. It verifies that the token
provided by the user upon authentication with the authentication provider contains at least one of the claims specified
in the filter.

	a: Path("/") -> oauthOidcAnyClaims("https://accounts.identity-provider.com", "some-client-id", "some-client-secret", "http://callback.com/auth/provider/callback", "scope1", "scope2") -> "https://internal.example.org" ;

OpenID - oauthOidcAllClaims filter
The filter oauthOidcAnyClaims is a filter for OAuth Implicit Flow authentication scheme for users. It verifies that the token
provided by the user upon authentication with the authentication provider contains all of the claims specified
in the filter.

	a: Path("/") -> oauthOidcAllClaims("https://accounts.identity-provider.com", "some-client-id", "some-client-secret", "http://callback.com/auth/provider/callback", "scope1", "scope2") -> "https://internal.example.org";

OAuth - auditLog() filter

The filter auditLog allows you to have an audit log for all
requests. This filter should be always set, before checking with auth
filters. To see only permitted access, you can set the auditLog()
filter after the auth filter.

    a: Path("/only-allowed-audit-log") -> oauthTokeninfoAnyScope("bar-w") -> auditLog() -> "https://internal.example.org/";
    b: Path("/all-access-requests-audit-log") -> auditLog() -> oauthTokeninfoAnyScope("foo-r") -> "https://internal.example.org/";

Webhook - webhook() filter

The filter webhook allows you to have a custom authentication and
authorization endpoint for a route.

    a: Path("/only-allowed-by-webhook") -> webhook("https://custom-webhook.example.org/auth") -> "https://protected-backend.example.org/";

Forward Token - forwardToken() filter

The filter is used to forward the result of token introspection or token info to the backend.

	a: Path("/tokeninfo-protected") -> oauthTokeninfoAnyScope("uid") -> forwardToken("X-Tokeninfo-Forward") -> "https://internal.example.org";
	b: Path("tokenintrospection-protected") -> oauthTokenintrospectionAnyKV("uid") -> forwardToken("X-Tokenintrospection-Forward") -> "http://internal.example.org";

*/
package auth
