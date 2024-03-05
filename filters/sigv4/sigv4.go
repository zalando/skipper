package sigv4

import (
	"bytes"
	"io"

	"github.com/zalando/skipper/filters"
)

type sigV4Spec struct{}
type sigV4Filter struct {
	bodySizeToBeRead int
	body             []byte
}

func New() filters.Spec {
	return &sigV4Spec{}
}

func (*sigV4Spec) Name() string {
	return filters.TLSName
}

func (c *sigV4Spec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	bodySizeToBeRead := args[0]

	bodySizeInInt, ok := bodySizeToBeRead.(int)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &sigV4Filter{
		bodySizeToBeRead: bodySizeInInt,
		body:             make([]byte, bodySizeInInt), // avoid resizing underlying array during run time
	}, nil
}

// only reads until the copiedBody is full
func copyBody(copiedBody []byte, body io.ReadCloser, maximumBodySize int) {
	buf := bytes.NewBuffer(copiedBody)
	temp := make([]byte, 8000) // assume that we read in 8kb chunks
	var n int32
	for n, err := body.Read(temp); err == nil; {
		buf.Write(temp[0:n])
		if buf.Len() >= maximumBodySize {
			n = 0
			break
		}
	}
	buf.Write(temp[0:n])
}

func signRequestWithoutBody(ctx filters.FilterContext) {

}

func signRequestWithBody(ctx filters.FilterContext, body []byte) {

}

/*
sigV4Filter is a request filter that signs the request.
In case a is non empty body is present in request,
the body is read to the maximum of bodySizeToBeRead value in 8kb chunks
and signed. The body is later reassigned to request. Operators should ensure
that bodySizeToBeRead is not more than the memory limit of skipper after taking
into accountthe concurrent requests.
*/
func (f *sigV4Filter) Request(ctx filters.FilterContext) {
	if ctx.Request().Body == nil {
		signRequestWithoutBody(ctx)
	} else {
		copyBody(f.body, ctx.Request().Body, f.bodySizeToBeRead)
		signRequestWithBody(ctx, f.body)
	}
}

func (f *sigV4Filter) Response(ctx filters.FilterContext) {}
