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
query string to do the request:
?access_token=<access-token-from-authorization-header>.

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


OAuth - auditLog() filter

The filter auditLog allows you to have an audit log for all
requests. This filter should be always set, before checking with auth
filters. To see only permitted access, you can set the auditLog()
filter after the auth filter.

    a: Path("/only-allowed-audit-log") -> oauthTokeninfoAnyScope("bar-w") -> auditLog() -> "https://internal.example.org/";
    b: Path("/all-access-requests-audit-log") -> auditLog() -> oauthTokeninfoAnyScope("foo-r") -> "https://internal.example.org/";

*/
package auth
