/*
Package diag provides a set of network throttling filters for diagnostic purpose.

The filters enable adding artificial latency, limiting bandwidth or chunking responses with custom chunk size
and delay. This throttling can be applied to the proxy responses or to the outgoing backend requests. An
additional filter, randomContent, can be used to generate response with random text of specified length.
*/
package diag

import (
	"io"
	"math/rand"
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/serve"
)

const defaultChunkSize = 512

const (
	RandomName           = "randomContent"
	LatencyName          = "latency"
	ChunksName           = "chunks"
	BandwidthName        = "bandwidth"
	BackendLatencyName   = "backendLatency"
	BackendBandwidthName = "backendBandwidth"
	BackendChunksName    = "backendChunks"
)

type throttleType int

const (
	latency throttleType = iota
	bandwidth
	chunks
	backendLatency
	backendBandwidth
	backendChunks
)

type random struct {
	len int
}

type throttle struct {
	typ       throttleType
	chunkSize int
	delay     time.Duration
}

var randomChars = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")

func kbps2bpms(kbps float64) float64 {
	return kbps * 1024 / 1000
}

// NewRandom creates a filter specification whose filter instances can be used
// to respond to requests with random text of specified length. It expects the
// the byte length of the random response to be generated as an argument.
// Eskip example:
//
// 	* -> randomContent(2048) -> <shunt>;
//
func NewRandom() filters.Spec { return &random{} }

// NewLatency creates a filter specification whose filter instances can be used
// to add additional latency to responses. It expects the latency in milliseconds
// as an argument. It always adds this value in addition to the natural latency,
// and does not do any adjustments. Eskip example:
//
// 	* -> latency(120) -> "https://www.example.org";
//
func NewLatency() filters.Spec { return &throttle{typ: latency} }

// NewBandwidth creates a filter specification whose filter instances can be used
// to maximize the bandwidth of the responses. It expects the bandwidth in
// kbyte/sec as an argument.
//
// 	* -> bandwidth(30) -> "https://www.example.org";
//
func NewBandwidth() filters.Spec { return &throttle{typ: bandwidth} }

// NewChunks creates a filter specification whose filter instances can be used
// set artificial delays in between response chunks. It expects the byte length
// of the chunks and the delay milliseconds.
//
// 	* -> chunks(1024, 120) -> "https://www.example.org";
//
func NewChunks() filters.Spec { return &throttle{typ: chunks} }

// NewBackendLatency is the equivalent of NewLatency but for outgoing backend
// responses. Eskip example:
//
// 	* -> backendLatency(120) -> "https://www.example.org";
//
func NewBackendLatency() filters.Spec { return &throttle{typ: backendLatency} }

// NewBackendBandwidth is the equivalent of NewBandwidth but for outgoing backend
// responses. Eskip example:
//
// 	* -> backendBandwidth(30) -> "https://www.example.org";
//
func NewBackendBandwidth() filters.Spec { return &throttle{typ: backendBandwidth} }

// NewBackendChunks is the equivalent of NewChunks but for outgoing backend
// responses. Eskip example:
//
// 	* -> backendChunks(1024, 120) -> "https://www.example.org";
//
func NewBackendChunks() filters.Spec { return &throttle{typ: backendChunks} }

func (r *random) Name() string { return RandomName }

func (r *random) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	if l, ok := args[0].(float64); ok {
		return &random{int(l)}, nil
	} else {
		return nil, filters.ErrInvalidFilterParameters
	}
}

func (r *random) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	for n := 0; n < r.len; {
		l := defaultChunkSize
		if n+l > r.len {
			l = r.len - n
		}

		b := make([]byte, l)
		for i := 0; i < l; i++ {
			b[i] = randomChars[rand.Intn(len(randomChars))]
		}

		ni, err := w.Write(b)
		if err != nil {
			log.Error(err)
			return
		}

		n += ni
	}
}

func (r *random) Request(ctx filters.FilterContext) {
	serve.ServeHTTP(ctx, r)
}

func (r *random) Response(ctx filters.FilterContext) {}

func (t *throttle) Name() string {
	switch t.typ {
	case latency:
		return LatencyName
	case bandwidth:
		return BandwidthName
	case chunks:
		return ChunksName
	case backendLatency:
		return BackendLatencyName
	case backendBandwidth:
		return BackendBandwidthName
	case backendChunks:
		return BackendChunksName
	default:
		panic("invalid throttle type")
	}
}

func parseLatencyArgs(args []interface{}) (int, time.Duration, error) {
	if len(args) != 1 {
		return 0, 0, filters.ErrInvalidFilterParameters
	}

	if msec, ok := args[0].(float64); ok && msec >= 0 {
		return 0, time.Duration(msec) * time.Millisecond, nil
	} else {
		return 0, 0, filters.ErrInvalidFilterParameters
	}
}

func parseBandwidthArgs(args []interface{}) (int, time.Duration, error) {
	if len(args) != 1 {
		return 0, 0, filters.ErrInvalidFilterParameters
	}

	kbps, ok := args[0].(float64)
	if !ok || kbps <= 0 {
		return 0, 0, filters.ErrInvalidFilterParameters
	}

	bpms := kbps2bpms(kbps)
	return defaultChunkSize, time.Duration(float64(defaultChunkSize)/bpms) * time.Millisecond, nil
}

func parseChunksArgs(args []interface{}) (int, time.Duration, error) {
	if len(args) != 2 {
		return 0, 0, filters.ErrInvalidFilterParameters
	}

	size, ok := args[0].(float64)
	if !ok || size <= 0 {
		return 0, 0, filters.ErrInvalidFilterParameters
	}

	msec, ok := args[1].(float64)
	if !ok || msec < 0 {
		return 0, 0, filters.ErrInvalidFilterParameters
	}

	return int(size), time.Duration(msec) * time.Millisecond, nil
}

func (t *throttle) CreateFilter(args []interface{}) (filters.Filter, error) {
	var (
		chunkSize int
		delay     time.Duration
		err       error
	)

	switch t.typ {
	case latency, backendLatency:
		chunkSize, delay, err = parseLatencyArgs(args)
	case bandwidth, backendBandwidth:
		chunkSize, delay, err = parseBandwidthArgs(args)
	case chunks, backendChunks:
		chunkSize, delay, err = parseChunksArgs(args)
	default:
		panic("invalid throttle type")
	}

	if err != nil {
		return nil, err
	}

	return &throttle{t.typ, chunkSize, delay}, nil
}

func (t *throttle) goThrottle(in io.ReadCloser, close bool) io.ReadCloser {
	if t.chunkSize <= 0 {
		if t.delay > 0 {
			time.Sleep(t.delay)
		}

		return in
	}

	r, w := io.Pipe()

	time.Sleep(t.delay)
	go func() {
		var err error
		defer func() {
			w.CloseWithError(err)
			if close {
				in.Close()
			}
		}()

		b := make([]byte, defaultChunkSize)
		for err == nil {
			n := 0

			var start time.Time
			switch t.typ {
			case bandwidth, backendBandwidth:
				start = time.Now()
			}

			for n < t.chunkSize {
				ni := 0
				eof := false

				bi := b
				if t.chunkSize-n < len(bi) {
					bi = bi[:t.chunkSize-n]
				}

				ni, err = in.Read(bi)
				if err == io.EOF {
					eof = true
					err = nil
				}

				if err != nil {
					break
				}

				ni, err = w.Write(bi[:ni])
				if err != nil {
					break
				}

				n += ni

				if eof {
					err = io.EOF
					break
				}
			}

			if err == nil {
				delay := t.delay

				switch t.typ {
				case bandwidth, backendBandwidth:
					delay -= time.Now().Sub(start)
				}

				if delay >= 0 {
					time.Sleep(t.delay)
				}
			}
		}
	}()

	return r
}

func (t *throttle) Request(ctx filters.FilterContext) {
	switch t.typ {
	case latency, bandwidth, chunks:
		return
	}

	req := ctx.Request()
	req.Body = t.goThrottle(req.Body, false)
}

func (t *throttle) Response(ctx filters.FilterContext) {
	switch t.typ {
	case backendLatency, backendBandwidth, backendChunks:
		return
	}

	rsp := ctx.Response()
	rsp.Body = t.goThrottle(rsp.Body, true)
}
