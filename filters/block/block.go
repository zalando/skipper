package block

import (
	"errors"

	"github.com/zalando/skipper/filters"
)

var (
	ErrClosed  = errors.New("reader closed")
	ErrBlocked = errors.New("blocked string match found in body")
)

type blockSpec struct {
	MaxMatcherBufferSize uint64
}

type block struct {
	match             []string
	maxEditorBuffer   uint64
	maxBufferHandling maxBufferHandling
}

func NewBlockFilter(maxMatcherBufferSize uint64) filters.Spec {
	return &blockSpec{
		MaxMatcherBufferSize: maxMatcherBufferSize,
	}
}

func (*blockSpec) Name() string {
	return filters.BlockName
}

func (bs *blockSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	sargs := make([]string, 0, len(args))
	for _, w := range args {
		switch v := w.(type) {
		case string:
			sargs = append(sargs, string(v))

		default:
			return nil, filters.ErrInvalidFilterParameters
		}
	}

	b := &block{
		match:             sargs,
		maxBufferHandling: maxBufferBestEffort,
		maxEditorBuffer:   bs.MaxMatcherBufferSize,
	}

	return *b, nil
}

func (b block) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	if req.ContentLength == 0 {
		return
	}

	req.Body = newMatcher(
		req.Body,
		b.match,
		b.maxEditorBuffer,
		b.maxBufferHandling,
	)
}

func (block) Response(filters.FilterContext) {}
