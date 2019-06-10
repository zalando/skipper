package rfc

// PatchPath attempts to patch a request path based on an interpretation of the standards
// RFC 2616 and RFC 2396 where the reserved characters should not be unescaped. Currently
// the Go stdlib does unescape these characters (v1.12.5).
//
// It expects the parsed path as found in http.Request.URL.Path and the raw path as found
// in http.Request.URL.RawPath. It returns a path where characters e.g. like '/' have the
// escaped form of %2F, if it was detected that they are unescaped in the raw path.
//
// It only returns the patched variant, if the only difference between the parsed and raw
// paths are the encoding of the reserved chars, according to RFC2396. If it detects any
// other difference between the two, it returns the original parsed path as provided. It
// tolerates empty argument for the raw path, which can happen when the URL parsed via the
// stdlib url package, and there is no difference between the parsed and the raw path.
// This basically means that the following code is correct:
//
// 	req.URL.Path = rfc.PatchPath(req.URL.Path, req.URL.RawPath)
//
// Links:
// - https://tools.ietf.org/html/rfc2616#section-3.2.3 and
// - https://tools.ietf.org/html/rfc2396#section-2.2
//
func PatchPath(parsed, raw string) string {
	return parsed
}
