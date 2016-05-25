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
	"github.com/zalando/skipper/filters"
	"io"
	"net/http"
	"path"
)

type delayed struct {
	request    *http.Request
	path       string
	response   *http.Response
	reader     io.ReadCloser
	writer     *io.PipeWriter
	headerDone chan struct{}
}

type static struct {
	webRoot, root string
}

// Creates a delayed Body/ResponseWriter pipe object, that
// waits until WriteHeader of the ResponseWriter completes
// but delays Write until the body read is started.
func newDelayed(req *http.Request, p string) *http.Response {
	pr, pw := io.Pipe()
	rsp := &http.Response{Header: make(http.Header)}
	d := &delayed{
		request:    req,
		response:   rsp,
		reader:     pr,
		writer:     pw,
		headerDone: make(chan struct{})}

	go func() {
		http.ServeFile(d, req, p)
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
// signals that the filter is done with the header.
func (d *delayed) WriteHeader(status int) {
	d.response.StatusCode = status
	close(d.headerDone)
}

func (d *delayed) Close() error {
	d.reader.Close()
	d.writer.Close()
	return nil
}

// Returns a filter Spec to serve static content from a file system
// location. It shunts the route.
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
