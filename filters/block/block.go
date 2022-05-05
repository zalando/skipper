package block

import (
	"errors"

	"github.com/zalando/skipper/filters"
)

const (
	defaultMaxBufferSize = 4096
)

var (
	ErrClosed  = errors.New("reader closed")
	ErrBlocked = errors.New("blocked string match found in body")
)

type blockSpec struct{}

type block struct {
	match             []string
	maxEditorBuffer   int
	maxBufferHandling maxBufferHandling
}

func parseMaxBufferHandling(h interface{}) (maxBufferHandling, error) {
	switch h {
	case "best-effort":
		return maxBufferBestEffort, nil
	case "abort":
		return maxBufferAbort, nil
	default:
		return 0, filters.ErrInvalidFilterParameters
	}
}

func NewBlockFilter() filters.Spec {
	return &blockSpec{}
}

func (*blockSpec) Name() string {
	return filters.BlockName
}

func (*blockSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
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
	}

	return *b, nil
}

func (b block) Request(ctx filters.FilterContext) {
	req := ctx.Request()
	if req.ContentLength == 0 {
		return
	}
	println(req.Body)
	req.Body = newMatcher(
		req.Body,
		b.match,
		b.maxEditorBuffer,
		b.maxBufferHandling,
	)
}

func (block) Response(filters.FilterContext) {}

// type blockBuffer struct {
// 	input         io.ReadCloser
// 	closed        bool
// 	maxBufferSize int
// 	match         []string
// }

// func newBlockBuffer(rc io.ReadCloser, match []string) *blockBuffer {
// 	return &blockBuffer{
// 		input:  rc,
// 		match:  match,
// 		closed: false,
// 	}
// }
// func (bmb *blockBuffer) Read(p []byte) (int, error) {
// 	// println("len(p)", len(p))
// 	if bmb.closed {
// 		// println("closed")
// 		return 0, ErrClosed
// 	}
// 	n, err := bmb.input.Read(p)
// 	if err != nil && err != io.EOF {
// 		log.Errorf("blockBuffer: Failed to read body: %v", err)
// 		// println("err not EOF")
// 		return 0, err
// 	}

// 	for _, s := range bmb.match {
// 		if bytes.Contains(p, []byte(s)) {
// 			p = nil
// 			log.Errorf("Content blocked: %v", err)
// 			return 0, ErrBlocked
// 		}
// 	}

// 	// println("END")
// 	return n, err
// }

// func (bmb *blockBuffer) Close() error {
// 	if bmb.closed {
// 		return nil
// 	}
// 	bmb.closed = true
// 	return bmb.input.Close()
// }
