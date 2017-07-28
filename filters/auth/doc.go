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

OAuth - Mechanism

The auth filter takes the incoming request, and tries to extract the Bearer token from the Authorization header. Then it validates against a configured service. Depending on the settings, it also can check if the owner of the token belongs to a specific OAuth2 realm, and it can check if it has at least one of the predefined scopes, or belongs to a certain team. If any of the expectations are not met, it doesn't forward the request to the target endpoint, but returns with status 401.

When team checking is configured, Skoap makes an additional request to the configured team service before forwarding the request, to get the teams of the owner of the token.

As additional features, the package also supports dropping the incoming Authorization header, replacing it with basic authorization. It also supports simple audit logging.

OAuth - Provider Configuration

-auth-service
-group-service

OAuth - auth() filter

OAuth - authGroup() filter

OAuth - Example
*/
package auth
