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

type delayedBody struct {
	request    *http.Request
	path       string
	response   *http.Response
	reader     io.ReadCloser
	writer     io.WriteCloser
	headerDone chan struct{}
}

type static struct {
	webRoot, root string
}

// creates a delayed body/response writer pipe object, that
// waits until WriteHeader of the response writer completes
// and delayes Write, until the body read is started
func newDelayed(req *http.Request, p string) *http.Response {
	println(p)
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

func (b *delayedBody) Read(data []byte) (int, error)  { return b.reader.Read(data) }
func (b *delayedBody) Header() http.Header            { return b.response.Header }
func (b *delayedBody) Write(data []byte) (int, error) { return b.writer.Write(data) }

func (b *delayedBody) WriteHeader(status int) {
	b.response.StatusCode = status
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
