// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package builtin

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"io"
	"net/http"
	"path"
	"strconv"
)

type delayedBody struct {
	request       *http.Request
	path          string
	response      *http.Response
	reader        io.ReadCloser
	writer        *io.PipeWriter
	contentLength int
	written       int
	headerDone    chan struct{}
}

type static struct {
	webRoot, root string
}

// creates a delayed body/response writer pipe object, that
// waits until WriteHeader of the response writer completes
// and delayes Write, until the body read is started
func newDelayed(req *http.Request, p string) *http.Response {
	pr, pw := io.Pipe()
	rsp := &http.Response{Header: make(http.Header)}
	db := &delayedBody{
		request:    req,
		path:       p,
		response:   rsp,
		reader:     pr,
		writer:     pw,
		headerDone: make(chan struct{})}
	go http.ServeFile(db, db.request, db.path)
	<-db.headerDone
	rsp.Body = db
	return rsp
}

func (b *delayedBody) Read(data []byte) (int, error) { return b.reader.Read(data) }
func (b *delayedBody) Header() http.Header           { return b.response.Header }

// implements http.ResponseWriter.Write, with the assumption that
// Content-Length is always set in advance.
func (b *delayedBody) Write(data []byte) (int, error) {
	if b.request.Method == "HEAD" || b.response.StatusCode >= http.StatusMultipleChoices {
		return 0, nil
	}

	n, err := b.writer.Write(data)
	if err != nil {
		return n, err
	}

	// pipe won't forward EOF, unless explicityly signaled
	// not signaled when Content-Encoding is set
	if b.response.Header.Get("Content-Encoding") == "" {
		b.written += n
		if b.written >= b.contentLength {
			b.writer.CloseWithError(io.EOF)
		}
	}

	return n, err
}

// implements http.ResponseWriter.WriteHeader.
// makes sure that the pipe is closed when Content-Length is
// not set, with the exception of Content-Encoding is set.
func (b *delayedBody) WriteHeader(status int) {
	b.response.StatusCode = status

	// no content on HEAD or redirect (304, not modified)
	if b.request.Method == "HEAD" ||
		status >= http.StatusMultipleChoices && status < http.StatusBadRequest {

		b.writer.CloseWithError(io.EOF)
		close(b.headerDone)
		return
	}

	// write body and close the pipe in case of an error
	if status >= http.StatusBadRequest {
		close(b.headerDone)

		_, err := b.writer.Write([]byte(http.StatusText(status)))
		if err != nil {
			log.Error(err)
		}

		b.writer.CloseWithError(io.EOF)
		return
	}

	// pipe close not handled when Content-Encoding is set
	if b.response.Header.Get("Content-Encoding") != "" {
		// currently no good option for this, but it shouldn't happen
		// based on the http.ServeFile
		close(b.headerDone)
		return
	}

	// take the expected Content-Length. If fails, close the pipe.
	cl, err := strconv.Atoi(b.response.Header.Get("Content-Length"))
	if cl == 0 || err != nil {
		if err != nil && b.response.Header.Get("Content-Length") != "" {
			log.Error(err)
		}

		b.writer.CloseWithError(io.EOF)
		close(b.headerDone)
		return
	}

	b.contentLength = cl
	close(b.headerDone)
}

func (b *delayedBody) Close() error {
	b.reader.Close()
	b.writer.Close()
	return nil
}

// Returns a filter Spec to serve static content from a file system
// location. Marks the request as served.
//
// Filter instances of this specification expect two parameters: a
// request path prefix and a local directory path. When processing a
// request, it clips the prefix from the request path, and appends the
// rest of the path to the directory path. Then, it uses the resulting
// path to serve static content from the file system.
//
// Name: "static".
func NewStatic() filters.Spec { return &static{} }

// "static"
func (spec *static) Name() string { return StaticName }

// Creates instances of the static filter. Expects two parameters: request path
// prefix and file system root.
func (spec *static) CreateFilter(config []interface{}) (filters.Filter, error) {
	if len(config) != 2 {
		return nil, fmt.Errorf("invalid number of args: %d, expected 1", len(config))
	}

	webRoot, ok := config[0].(string)
	if !ok {
		return nil, fmt.Errorf("invalid parameter type, expected string for web root prefix")
	}

	root, ok := config[1].(string)
	if !ok {
		return nil, fmt.Errorf("invalid parameter type, expected string for path to root dir")
	}

	return &static{webRoot, root}, nil
}

// Serves content from the file system and marks the request served.
func (f *static) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	p := req.URL.Path

	if len(p) < len(f.webRoot) {
		ctx.Serve(&http.Response{StatusCode: http.StatusNotFound})
		return
	}

	ctx.Serve(newDelayed(req, path.Join(f.root, p[len(f.webRoot):])))
}

// Noop.
func (f *static) Response(filters.FilterContext) {}
