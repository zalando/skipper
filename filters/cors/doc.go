/*
Package cors implements the origin header for CORS.

# How It Works

The filter accepts an optional variadic list of acceptable origin parameters. If the input argument list is empty, the header
will always be set to '*' which means any origin is acceptable. Otherwise, the header is only set if the request contains
an Origin header and its value matches one of the elements in the input list. The header is only set on the response.

Usage

	corsOrigin()
	corsOrigin("https://www.example.org")
	corsOrigin("https://www.example.org", "http://localhost:9001")
*/
package cors
