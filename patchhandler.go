package skipper

import (
	"net/http"
	"regexp"
)

type wrapPatch struct {
	inner http.Handler
}

var hackRx = regexp.MustCompile("\\W\\Wjndi:")

func WrapPatch(h http.Handler) http.Handler {
	return &wrapPatch{inner: h}
}

func matchHack(s string) bool {
	return hackRx.MatchString(s)
}

func (p *wrapPatch) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if matchHack(r.RequestURI) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	for k, v := range r.Header {
		if matchHack(k) {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		for _, vi := range v {
			if matchHack(vi) {
				w.WriteHeader(http.StatusNotFound)
				return
			}
		}
	}

	p.inner.ServeHTTP(w, r)
}
