/*
Package serve provides a wrapper of net/http.Handler to be used as a filter.
*/
package serve

import (
	"io"
	"net/http"

	"github.com/zalando/skipper/filters"
)

type pipedResponse struct {
	response   *http.Response
	reader     *io.PipeReader
	writer     *io.PipeWriter
	headerDone chan struct{}
}

// ServeHTTP creates a response from a handler and a request.
//
// It calls the handler's ServeHTTP method with an internal response
// writer that shares the status code, headers and the response body
// with the returned response. It blocks until the handler calls the
// response writer's WriteHeader, or starts writing the body, or
// returns. The written body is not buffered, but piped to the returned
// response's body.
//
// Example, a simple file server:
//
//	var handler = http.StripPrefix(webRoot, http.FileServer(http.Dir(root)))
//
//	func (f *myFilter) Request(ctx filters.FilterContext) {
//		serve.ServeHTTP(ctx, handler)
//	}
func ServeHTTP(ctx filters.FilterContext, h http.Handler) {
	rsp := &http.Response{Header: make(http.Header)}
	r, w := io.Pipe()
	d := &pipedResponse{
		response:   rsp,
		reader:     r,
		writer:     w,
		headerDone: make(chan struct{})}

	req := ctx.Request()
	go func() {
		h.ServeHTTP(d, req)
		select {
		case <-d.headerDone:
		default:
			d.WriteHeader(http.StatusOK)
		}

		w.CloseWithError(io.EOF)
	}()

	<-d.headerDone
	rsp.Body = d
	ctx.Serve(rsp)
}

func (d *pipedResponse) Read(data []byte) (int, error) { return d.reader.Read(data) }
func (d *pipedResponse) Header() http.Header           { return d.response.Header }

// Implements http.ResponseWriter.Write. When WriteHeader was
// not called before Write, it calls it with the default 200
// status code.
func (d *pipedResponse) Write(data []byte) (int, error) {
	select {
	case <-d.headerDone:
	default:
		d.WriteHeader(http.StatusOK)
	}

	return d.writer.Write(data)
}

// It sets the status code for the outgoing response, and
// signals that the header is done.
func (d *pipedResponse) WriteHeader(status int) {
	d.response.StatusCode = status
	close(d.headerDone)
}

func (d *pipedResponse) Close() error {
	return d.reader.Close()
}
