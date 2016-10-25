/*
Package auth implements the basic auth for headers based on "https://github.com/abbot/go-http-auth".

How It Works

The filter accepts two parameters, the first mandatory one is the path to the htpasswd file usually used with Apache or nginx. The second one is the optional realm name that will be displayed in the browser.
Each incoming request will be validated against the password file, for more information which formats are currently
supported check "https://github.com/abbot/go-http-auth".
Assuming you are going to use the MD5 version new entries can be generated like

	htpasswd -nbm myName myPassword

Usage

	basicAuth("/path/to/htpasswd")
	basicAuth("/path/to/htpasswd", "My Website")
*/
package auth
