package rfc

const escapeLength = 3 // always, because we only handle the below cases

const (
	semicolon    = ';'
	slash        = '/'
	questionMark = '?'
	colon        = ':'
	at           = '@'
	and          = '&'
	eq           = '='
	plus         = '+'
	dollar       = '$'
	comma        = ','
)

// https://tools.ietf.org/html/rfc2396#section-2.2
func unescape(seq []byte) (byte, bool) {
	switch string(seq) {
	case "%3B", "%3b":
		return semicolon, true
	case "%2F", "%2f":
		return slash, true
	case "%3F", "%3f":
		return questionMark, true
	case "%3A", "%3a":
		return colon, true
	case "%40":
		return at, true
	case "%26":
		return and, true
	case "%3D", "%3d":
		return eq, true
	case "%2B", "%2b":
		return plus, true
	case "%24":
		return dollar, true
	case "%2C", "%2c":
		return comma, true
	default:
		return 0, false
	}
}

// PatchPath attempts to patch a request path based on an interpretation of the standards
// RFC 2616 and RFC 2396 where the reserved characters should not be unescaped. Currently
// the Go stdlib does unescape these characters (v1.12.5).
//
// It expects the parsed path as found in http.Request.URL.Path and the raw path as found
// in http.Request.URL.RawPath. It returns a path where characters e.g. like '/' have the
// escaped form of %2F, if it was detected that they are unescaped in the raw path.
//
// It only returns the patched variant, if the only difference between the parsed and raw
// paths are the encoding of the chars, according to RFC 2396. If it detects any other
// difference between the two, it returns the original parsed path as provided. It
// tolerates an empty argument for the raw path, which can happen when the URL parsed via
// the stdlib url package, and there is no difference between the parsed and the raw path.
// This basically means that the following code is correct:
//
// 	req.URL.Path = rfc.PatchPath(req.URL.Path, req.URL.RawPath)
//
// Links:
// - https://tools.ietf.org/html/rfc2616#section-3.2.3 and
// - https://tools.ietf.org/html/rfc2396#section-2.2
//
func PatchPath(parsed, raw string) string {
	p, r := []byte(parsed), []byte(raw)
	patched := make([]byte, 0, len(r))
	var (
		escape    bool
		seq       []byte
		unescaped byte
		handled   bool
		doPatch   bool
		modified  bool
		pi        int
	)

	for i := 0; i < len(r); i++ {
		c := r[i]
		escape = c == '%'
		modified = pi >= len(p) || !escape && p[pi] != c
		if modified {
			doPatch = false
			break
		}

		if !escape {
			patched = append(patched, p[pi])
			pi++
			continue
		}

		if len(r) < i+escapeLength {
			doPatch = false
			break
		}

		seq = r[i : i+escapeLength]
		i += escapeLength - 1
		unescaped, handled = unescape(seq)
		if !handled {
			patched = append(patched, p[pi])
			pi++
			continue
		}

		modified = p[pi] != unescaped
		if modified {
			doPatch = false
			break
		}

		patched = append(patched, seq...)
		doPatch = true
		pi++
	}

	if !doPatch {
		return parsed
	}

	modified = pi < len(p)
	if modified {
		return parsed
	}

	return string(patched)
}
