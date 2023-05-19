package block

import (
	"bytes"
	"encoding/hex"
	"errors"

	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/net"
)

var (
	ErrClosed = errors.New("reader closed")
)

type blockSpec struct {
	MaxMatcherBufferSize uint64
	hex                  bool
}

type toBlockKeys struct{ Str []byte }

func (b toBlockKeys) String() string {
	return string(b.Str)
}

type block struct {
	toblockList       []toBlockKeys
	maxEditorBuffer   uint64
	maxBufferHandling net.MaxBufferHandling
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

	sargs := make([]toBlockKeys, 0, len(args))
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
			sargs = append(sargs, toBlockKeys{Str: a})
		} else {
			sargs = append(sargs, toBlockKeys{Str: []byte(v)})
		}
	}

	return &block{
		toblockList:       sargs,
		maxBufferHandling: net.MaxBufferBestEffort,
		maxEditorBuffer:   bs.MaxMatcherBufferSize,
	}, nil
}

func blockMatcher(matches []toBlockKeys) func(b []byte) (int, error) {
	return func(b []byte) (int, error) {
		for _, s := range matches {
			s := s
			println("blockMatcher:", string(b), len(string(b)), "contains?:", string(s.Str))
			if bytes.Contains(b, s.Str) {
				b = nil
				return 0, net.ErrBlocked
			}
		}
		return len(b), nil
	}
}

func (b *block) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	if req.ContentLength == 0 {
		return
	}
	// fix filter chaining - https://github.com/zalando/skipper/issues/2605
	ctx.Request().Header.Del("Content-Length")
	ctx.Request().ContentLength = -1

	req.Body = net.WrapBodyWithOptions(
		req.Context(),
		net.BodyOptions{
			MaxBufferHandling: b.maxBufferHandling,
			ReadBufferSize:    b.maxEditorBuffer,
		},
		blockMatcher(b.toblockList),
		req.Body)
}

func (*block) Response(filters.FilterContext) {}
