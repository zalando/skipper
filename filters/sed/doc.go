/*
Package sed provides stream editor filters for request and response payload.

# Filter sed

Example:

	editorRoute: * -> sed("foo", "bar") -> "https://www.example.org";

Example with larger max buffer:

	editorRoute: * -> sed("foo", "bar", 64000000) -> "https://www.example.org";

This filter expects a regexp pattern and a replacement string as arguments.
During the streaming of the response body, every occurrence of the pattern will
be replaced with the replacement string. The editing doesn't happen right when
the filter is executed, only later when the streaming normally happens, after
all response filters were called.

The sed() filter accepts two optional arguments, the max editor buffer size in
bytes, and max buffer handling flag. The max buffer size, when set, defines how
much data can be buffered at a given time by the editor. The default value is
2MiB. The max buffer handling flag can take one of two values: "abort" or
"best-effort" (default). Setting "abort" means that the stream will be aborted
when reached the limit. Setting "best-effort", will run the replacement on the
available content, in case of certain patterns, this may result in content that
is different from one that would have been edited in a single piece. See more
details below.

The filter uses the go regular expression implementation:
https://github.com/google/re2/wiki/Syntax . Due to the streaming nature, matches
with zero length are ignored.

# Memory handling and limitations

In order to avoid unbound buffering of unprocessed data, the sed* filters need to
apply some limitations. Some patterns, e.g. `.*` would allow to match the complete
payload, and it could result in trying to buffer it all and potentially causing
running out of available memory. Similarly, in case of certain expressions, when
they don't match, it's impossible to tell if they would match without reading more
data from the source, and so would potentially need to buffer the entire payload.

To prevent too high memory usage, the max buffer size is limited in case of each
variant of the filter, by default to 2MiB, which is the same limit as the one we
apply when reading the request headers by default. When the limit is reached, and
the buffered content matches the pattern, then it is processed by replacing it,
when it doesn't match the pattern, then it is forwarded unchanged. This way, e.g.
`sed(".*", "")` can be used safely to consume and discard the payload.

As a result of this, with large payloads, it is possible that the resulting content
will be different than if we had run the replacement on the entire content at once.
If we have enough preliminary knowledge about the payload, then it may be better to
use the delimited variant of the filters, e.g. for line based editing.

If the max buffer handling is set to "abort", then the stream editing is stopped
and the rest of the payload is dropped.

# Filter sedDelim

Like sed(), but it expects an additional argument, before the optional max buffer
size argument, that is used to delimit chunks to be processed at once. The pattern
replacement is executed only within the boundaries of the chunks defined by the
delimiter, and matches across the chunk boundaries are not considered.

	editorRoute: * -> sedDelim("foo", "bar", "\n") -> "https://www.example.org";

# Filter sedRequest

Like sed(), but for the request content.

	editorRoute: * -> sedRequest("foo", "bar") -> "https://www.example.org";

# Filter sedRequestDelim

Like sedDelim(), but for the request content.

	editorRoute: * -> sedRequestDelim("foo", "bar", "\n") -> "https://www.example.org";
*/
package sed
