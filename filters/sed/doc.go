/*
Package sed provides stream editor filters for request and response payload.

The filter sed() expects a regexp pattern and a replacement string as arguments. During the streaming of the
response body, every occurence of the pattern will be replaced with the replacement string. The editing doesn't
happen right when the filter is executed, only later when the streaming normally happens, after all response
filters were called.

Mandatory arguments:

Optional arguments:

The sed() filter accepts an optional third argument, the max editor buffer size in bytes.
This argument limits how much data can be buffered at a given time by the editor. The default value is 2MiB. See
more details below.

The filter uses the go regular expression implementation: https://github.com/google/re2/wiki/Syntax

Example:

	* -> sed("foo", "bar") -> "https://www.example.org"

The filter sedDelim() is like sed(), but it expects an additional argument, before the optional max buffer size
argument, that is used to delimit chunks to be processed at once. The pattern replacement is executed only
within the boundaries of the chunks defined by the delimiter, and matches across the chunk boundaries are not
considered.

Example:

	* -> sedDelim("foo", "bar", "\n") -> "https://www.example.org"

The filter sedRequest() is like sed(), but for the request content.

Example:

	* -> sedRequest("foo", "bar") -> "https://www.example.org"

The filter sedRequestDelim() is like sedDelim(), but for the request content.

Example:

	* -> sedRequestDelim("foo", "bar", "\n") -> "https://www.example.org"

Memory handling and limitations

In order to avoid unbound buffering of unprocessed data, the sed* filters need to apply some limitations. Some
patterns, e.g. `.*` would allow to match the complete payload, and it could result in trying to buffer it all
and potentially causing running out of available memory. Similarly, in case of certain expressions, when they
don't match, it's impossible to tell if they would match without reading more data from the source, and so would
potentially need to buffer the entire payload.

To prevent too high memory usage, the max buffer size is limited in case of each variant of the filter, by
default to 2MiB, which is the same limit as the one we apply when reading the request headers by default. When
the limit is reached, and the buffered content matches the pattern, then it is processed by replacing it, when
it doesn't match the pattern, then it is forwarded unchanged. This way, e.g. `sed(".*", "")` can be used safely
to consume and discard the payload.

As a result of this, with large payloads, it is possible that the resulting content will be different than if we
had run the replacement on the entire content at once. If we have enough preliminary knowledge about the
payload, then it may be better to use the delimited variant of the filters, e.g. for line based editing.
*/
package sed
