package diag

import (
	"net/http"
	"time"

	"github.com/zalando/skipper/filters"
)

type tarpitSpec struct{}

type tarpit struct {
	d time.Duration
}

func NewTarpit() filters.Spec {
	return &tarpitSpec{}
}

func (t *tarpitSpec) Name() string {
	return filters.TarpitName
}

func (t *tarpitSpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}
	s, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return nil, filters.ErrInvalidFilterParameters
	}

	return &tarpit{d: d}, nil
}

func (t *tarpit) Request(ctx filters.FilterContext) {
	ctx.Serve(&http.Response{StatusCode: http.StatusOK, Body: &slowBlockingReader{d: t.d}})
}

func (*tarpit) Response(filters.FilterContext) {}

type slowBlockingReader struct {
	d time.Duration
}

func (r *slowBlockingReader) Read(p []byte) (int, error) {
	time.Sleep(r.d)
	n := copy(p, []byte(" "))
	return n, nil
}

func (r *slowBlockingReader) Close() error {
	return nil
}
