package block

import (
	"bytes"
	"encoding/hex"

	"github.com/zalando/skipper/filters"
	skpio "github.com/zalando/skipper/io"
	"github.com/zalando/skipper/metrics"
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
	maxBufferHandling skpio.MaxBufferHandling
	metrics           metrics.Metrics
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

func (bs *blockSpec) CreateFilter(args []any) (filters.Filter, error) {
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
		maxBufferHandling: skpio.MaxBufferBestEffort,
		maxEditorBuffer:   bs.MaxMatcherBufferSize,
		metrics:           metrics.Default,
	}, nil
}

func blockMatcher(m metrics.Metrics, matches []toBlockKeys) func(b []byte) (int, error) {
	return func(b []byte) (int, error) {
		for _, s := range matches {
			if bytes.Contains(b, s.Str) {
				m.IncCounter("blocked.requests")
				return 0, skpio.ErrBlocked
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

	req.Body = skpio.InspectReader(
		req.Context(),
		skpio.BufferOptions{
			MaxBufferHandling: b.maxBufferHandling,
			ReadBufferSize:    b.maxEditorBuffer,
		},
		blockMatcher(b.metrics, b.toblockList),
		req.Body)
}

func (*block) Response(filters.FilterContext) {}
