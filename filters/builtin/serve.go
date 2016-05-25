package builtin

import (
	"io"
	"net/http"
)

type delayed struct {
	response   *http.Response
	reader     io.ReadCloser
	writer     *io.PipeWriter
	headerDone chan struct{}
}

// Creates a response from a handler and a request.
//
// It calls the handler's ServeHTTP method with an internal response
// writer that shares the status code, headers and the response body
// with the returned response.
//
// It blocks the handler calls the response writer's WriteHeader, or
// starts writing the body or returns.
//
// The written body is not buffered, but piped to the returned
// response's body.
func ServeResponse(req *http.Request, h http.Handler) *http.Response {
	pr, pw := io.Pipe()
	rsp := &http.Response{Header: make(http.Header)}
	d := &delayed{
		response:   rsp,
		reader:     pr,
		writer:     pw,
		headerDone: make(chan struct{})}

	go func() {
		h.ServeHTTP(d, req)
		select {
		case <-d.headerDone:
		default:
			d.WriteHeader(http.StatusOK)
		}

		pw.CloseWithError(io.EOF)
	}()

	<-d.headerDone
	rsp.Body = d
	return rsp
}

func (d *delayed) Read(data []byte) (int, error) { return d.reader.Read(data) }
func (d *delayed) Header() http.Header           { return d.response.Header }

// Implements http.ResponseWriter.Write. When WriteHeader was
// not called before Write, it calls it with the default 200
// status code.
func (d *delayed) Write(data []byte) (int, error) {
	select {
	case <-d.headerDone:
	default:
		d.WriteHeader(http.StatusOK)
	}

	return d.writer.Write(data)
}

// It sets the status code for the outgoing response, and
// signals that the header is done.
func (d *delayed) WriteHeader(status int) {
	d.response.StatusCode = status
	close(d.headerDone)
}

func (d *delayed) Close() error {
	d.reader.Close()
	d.writer.Close()
	return nil
}
