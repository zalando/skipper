package block

import (
	"encoding/hex"
	"errors"

	"github.com/zalando/skipper/filters"
)

var (
	ErrClosed = errors.New("reader closed")
)

type blockSpec struct {
	MaxMatcherBufferSize uint64
	hex                  bool
}

type block struct {
	toblockList       []toblockKeys
	maxEditorBuffer   uint64
	maxBufferHandling maxBufferHandling
}

// NewBlockFilter *deprecated* version of NewBlock
func NewBlockFilter(maxMatcherBufferSize uint64) filters.Spec {
	return NewBlock(maxMatcherBufferSize)
}

func NewBlock(maxMatcherBufferSize uint64) filters.Spec {
	return &blockSpec{
		MaxMatcherBufferSize: maxMatcherBufferSize,
	}
}

func NewBlockHex(maxMatcherBufferSize uint64) filters.Spec {
	return &blockSpec{
		MaxMatcherBufferSize: maxMatcherBufferSize,
		hex:                  true,
	}
}

func (bs *blockSpec) Name() string {
	if bs.hex {
		return filters.BlockHexName
	}
	return filters.BlockName
}

func (bs *blockSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	sargs := make([]toblockKeys, 0, len(args))
	for _, w := range args {
		v, ok := w.(string)
		if !ok {
			return nil, filters.ErrInvalidFilterParameters
		}
		if bs.hex {
			a, err := hex.DecodeString(v)
			if err != nil {
				return nil, err
			}
			sargs = append(sargs, toblockKeys{str: a})
		} else {
			sargs = append(sargs, toblockKeys{str: []byte(v)})
		}
	}

	b := &block{
		toblockList:       sargs,
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
		b.toblockList,
		b.maxEditorBuffer,
		b.maxBufferHandling,
	)
}

func (block) Response(filters.FilterContext) {}
