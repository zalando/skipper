package proxy

import (
	"io"
	"maps"
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"
)

type teeTie struct {
	r io.Reader
	w *io.PipeWriter
}

func (tt *teeTie) Read(b []byte) (int, error) {
	n, err := tt.r.Read(b)

	if err != nil && err != io.EOF {
		tt.w.CloseWithError(err)
		return n, err
	}

	if n > 0 {
		if _, werr := tt.w.Write(b[:n]); werr != nil {
			log.Error("tee: error while tee request", werr)
		}
	}

	if err == io.EOF {
		tt.w.Close()
	}

	return n, err
}

func (tt *teeTie) Close() error { return nil }

// Returns the cloned request and the tee body to be used on the main request.
func cloneRequestForSplit(u *url.URL, req *http.Request) (*http.Request, io.ReadCloser, error) {
	h := make(http.Header)
	maps.Copy(h, req.Header)

	var teeBody io.ReadCloser
	mainBody := req.Body

	if req.ContentLength != 0 {
		pr, pw := io.Pipe()
		teeBody = pr
		mainBody = &teeTie{mainBody, pw}
	}

	clone, err := http.NewRequest(req.Method, u.String(), teeBody)
	if err != nil {
		return nil, nil, err
	}

	clone.RequestURI = req.RequestURI
	clone.Header = h
	clone.ContentLength = req.ContentLength
	clone.RemoteAddr = req.RemoteAddr

	return clone, mainBody, nil
}
