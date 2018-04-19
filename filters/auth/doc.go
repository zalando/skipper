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

Set Basic - Outgoing Authentication Header

OAuth - Check Bearer Tokens

The auth filter takes the incoming request, and tries to extract the
Bearer token from the Authorization header. Then it validates against
a configured service. Depending on the settings, it also can check if
the owner of the token belongs to a specific OAuth2 realm, and it can
check if it has at least one of the predefined scopes. If any of the
expectations are not met, it doesn't forward the request to the target
endpoint, but returns with status 401.

As additional features, the package also supports audit logging.

OAuth - Provider Configuration

To enable OAuth2 filters you have to set the CLI argument
-token-url=<TokenURL>.  Scopes and realms are dependend on your OAuth2
infrastructure provider. Tokens can be OAuth2 or JWT as long as
TokenURL returns the data being checked.

OAuth - authAny() filter

The filter authAny allows access if the realm and one of the scopes
are satisfied by the request.

    Path("/uid") -> authAny("/employees", "uid") -> "https://internal.example.org/";
    Path("/") -> authAny("/employees", "uid", "bar") -> "https://internal.example.org/";

OAuth - authAll() filter

The filter authAll allows access if the realm and all of the scopes
are satisfied by the request.

    Path("/uid") -> authAll("/employees", "uid") -> "https://internal.example.org/";
    Path("/") -> authAll("/employees", "uid", "bar") -> "https://internal.example.org/";

OAuth - auditLog() filter

The filter auditLog allows you to have an audit log for all
requests. This filter should be always set, before checking with auth
filters. To see only permitted access, you can set the auditLog()
filter after the auth filter.

    Path("/only-allowed-audit-log") -> authAny("/employees", "uid") -> auditLog() -> "https://internal.example.org/";
    Path("/all-access-requests-audit-log") -> auditLog() -> authAny("/employees", "uid") -> "https://internal.example.org/";

*/
package auth
