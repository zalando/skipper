/*
Package diag provides a set of network throttling filters for diagnostic purpose.

The filters enable adding artificial latency, limiting bandwidth or chunking responses with custom chunk size
and delay. This throttling can be applied to the proxy responses or to the outgoing backend requests. An
additional filter, randomContent, can be used to generate response with random text of specified length.
*/
package diag

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/zalando/skipper/filters"
)

const defaultChunkSize = 512

const (
	// Deprecated, use filters.RandomContentName instead
	RandomName = filters.RandomContentName
	// Deprecated, use filters.RepeatContentName instead
	RepeatName = filters.RepeatContentName
	// Deprecated, use filters.LatencyName instead
	LatencyName = filters.LatencyName
	// Deprecated, use filters.ChunksName instead
	ChunksName = filters.ChunksName
	// Deprecated, use filters.BandwidthName instead
	BandwidthName = filters.BandwidthName
	// Deprecated, use filters.BackendLatencyName instead
	BackendLatencyName = filters.BackendLatencyName
	// Deprecated, use filters.BackendBandwidthName instead
	BackendBandwidthName = filters.BackendBandwidthName
	// Deprecated, use filters.BackendChunksName instead
	BackendChunksName = filters.BackendChunksName
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
	mu   sync.Mutex
	rand *rand.Rand
	len  int64
}

type (
	repeatSpec struct {
		hex bool
	}
	repeat struct {
		bytes []byte
		len   int64
	}
	repeatReader struct {
		bytes  []byte
		offset int
	}
)

type (
	wrapSpec struct {
		hex bool
	}
	wrap struct {
		prefix, suffix []byte
	}
	wrapReadCloser struct {
		io.Reader
		io.Closer
	}
)

type throttle struct {
	typ       throttleType
	chunkSize int
	delay     time.Duration
}

type distribution int

const (
	uniformRequestDistribution distribution = iota
	normalRequestDistribution
	uniformResponseDistribution
	normalResponseDistribution
)

type jitter struct {
	mean  time.Duration
	delta time.Duration
	typ   distribution
	sleep func(time.Duration)
}

var (
	// https://github.com/cilium/cilium/pull/32542/
	// randSrc is a source of pseudo-random numbers. It is seeded to the current time in
	// nanoseconds by default but can be reseeded in tests so they are deterministic.
	randSrc = rand.NewPCG(uint64(time.Now().UnixNano()), 0)
	randGen = rand.New(randSrc)
)

var randomChars = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")

func kbps2bpms(kbps float64) float64 {
	return kbps * 1024 / 1000
}

// NewRandom creates a filter specification whose filter instances can be used
// to respond to requests with random text of specified length. It expects the
// the byte length of the random response to be generated as an argument.
// Eskip example:
//
//	r: * -> randomContent(2048) -> <shunt>;
func NewRandom() filters.Spec { return &random{} }

// NewRepeat creates a filter specification whose filter instances can be used
// to respond to requests with a repeated text. It expects the text and
// the byte length of the response body to be generated as arguments.
// Eskip example:
//
//	r: * -> repeatContent("x", 100) -> <shunt>;
func NewRepeat() filters.Spec { return &repeatSpec{hex: false} }

// NewRepeatHex creates a filter specification whose filter instances can be used
// to respond to requests with a repeated bytes.
// It expects the bytes represented by the hexadecimal string of an even length and
// the byte length of the response body to be generated as arguments.
// Eskip example:
//
//	r: * -> repeatContentHex("0123456789abcdef", 16) -> <shunt>;
func NewRepeatHex() filters.Spec { return &repeatSpec{hex: true} }

// NewWrap creates a filter specification whose filter instances can be used
// to add prefix and suffix to the response.
// Eskip example:
//
//	r: * -> wrapContent("foo", "baz") -> inlineContent("bar") -> <shunt>;
func NewWrap() filters.Spec { return &wrapSpec{hex: false} }

// NewWrapHex creates a filter specification whose filter instances can be used
// to add prefix and suffix represented by the hexadecimal strings of an even length to the response.
// Eskip example:
//
//	r: * -> wrapContentHex("68657861", "6d616c") -> inlineContent("deci") -> <shunt>;
func NewWrapHex() filters.Spec { return &wrapSpec{hex: true} }

// NewLatency creates a filter specification whose filter instances can be used
// to add additional latency to responses. It expects the latency in milliseconds
// as an argument. It always adds this value in addition to the natural latency,
// and does not do any adjustments. Eskip example:
//
//	r: * -> latency(120) -> "https://www.example.org";
func NewLatency() filters.Spec { return &throttle{typ: latency} }

// NewBandwidth creates a filter specification whose filter instances can be used
// to maximize the bandwidth of the responses. It expects the bandwidth in
// kbyte/sec as an argument.
//
//	r: * -> bandwidth(30) -> "https://www.example.org";
func NewBandwidth() filters.Spec { return &throttle{typ: bandwidth} }

// NewChunks creates a filter specification whose filter instances can be used
// set artificial delays in between response chunks. It expects the byte length
// of the chunks and the delay milliseconds.
//
//	r: * -> chunks(1024, "120ms") -> "https://www.example.org";
func NewChunks() filters.Spec { return &throttle{typ: chunks} }

// NewBackendLatency is the equivalent of NewLatency but for outgoing backend
// requests. Eskip example:
//
//	r: * -> backendLatency(120) -> "https://www.example.org";
func NewBackendLatency() filters.Spec { return &throttle{typ: backendLatency} }

// NewBackendBandwidth is the equivalent of NewBandwidth but for outgoing backend
// requests. Eskip example:
//
//	r: * -> backendBandwidth(30) -> "https://www.example.org";
func NewBackendBandwidth() filters.Spec { return &throttle{typ: backendBandwidth} }

// NewBackendChunks is the equivalent of NewChunks but for outgoing backend
// requests. Eskip example:
//
//	r: * -> backendChunks(1024, 120) -> "https://www.example.org";
func NewBackendChunks() filters.Spec { return &throttle{typ: backendChunks} }

// NewUniformRequestLatency creates a latency for requests with uniform
// distribution. Example delay around 1s with +/-120ms.
//
//	r: * -> uniformRequestLatency("1s", "120ms") -> "https://www.example.org";
func NewUniformRequestLatency() filters.Spec { return &jitter{typ: uniformRequestDistribution} }

// NewNormalRequestLatency creates a latency for requests with normal
// distribution. Example delay around 1s with +/-120ms.
//
//	r: * -> normalRequestLatency("1s", "120ms") -> "https://www.example.org";
func NewNormalRequestLatency() filters.Spec { return &jitter{typ: normalRequestDistribution} }

// NewUniformResponseLatency creates a latency for responses with uniform
// distribution. Example delay around 1s with +/-120ms.
//
//	r: * -> uniformRequestLatency("1s", "120ms") -> "https://www.example.org";
func NewUniformResponseLatency() filters.Spec { return &jitter{typ: uniformResponseDistribution} }

// NewNormalResponseLatency creates a latency for responses with normal
// distribution. Example delay around 1s with +/-120ms.
//
//	r: * -> normalRequestLatency("1s", "120ms") -> "https://www.example.org";
func NewNormalResponseLatency() filters.Spec { return &jitter{typ: normalResponseDistribution} }

func (r *random) Name() string { return filters.RandomContentName }

func (r *random) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	if l, ok := args[0].(float64); ok {
		return &random{rand: randGen, len: int64(l)}, nil
	} else {
		return nil, filters.ErrInvalidFilterParameters
	}
}

func (r *random) Read(p []byte) (int, error) {
	for i := 0; i < len(p); i++ {
		p[i] = randomChars[r.rand.IntN(len(randomChars))]
	}
	return len(p), nil
}

func (r *random) Request(ctx filters.FilterContext) {
	ctx.Serve(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(io.LimitReader(r, r.len)),
	})
}

func (r *random) Response(ctx filters.FilterContext) {}

func (r *repeatSpec) Name() string {
	if r.hex {
		return filters.RepeatContentHexName
	} else {
		return filters.RepeatContentName
	}
}

func (r *repeatSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	text, ok := args[0].(string)
	if !ok || text == "" {
		return nil, filters.ErrInvalidFilterParameters
	}

	f := &repeat{}
	if r.hex {
		var err error
		f.bytes, err = hex.DecodeString(text)
		if err != nil {
			return nil, err
		}
	} else {
		f.bytes = []byte(text)
	}

	switch v := args[1].(type) {
	case float64:
		f.len = int64(v)
	case int:
		f.len = int64(v)
	case int64:
		f.len = v
	default:
		return nil, filters.ErrInvalidFilterParameters
	}
	if f.len < 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	return f, nil
}

func (r *repeat) Request(ctx filters.FilterContext) {
	ctx.Serve(&http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Length": []string{strconv.FormatInt(r.len, 10)}},
		Body:       io.NopCloser(io.LimitReader(&repeatReader{r.bytes, 0}, r.len)),
	})
}

func (r *repeatReader) Read(p []byte) (int, error) {
	n := copy(p, r.bytes[r.offset:])
	if n < len(p) {
		n += copy(p[n:], r.bytes[:r.offset])
		for n < len(p) {
			copy(p[n:], p[:n])
			n *= 2
		}
	}
	r.offset = (r.offset + len(p)) % len(r.bytes)
	return len(p), nil
}

func (r *repeat) Response(ctx filters.FilterContext) {}

func (w *wrapSpec) Name() string {
	if w.hex {
		return filters.WrapContentHexName
	} else {
		return filters.WrapContentName
	}
}

func (w *wrapSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	if len(args) != 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	prefix, ok := args[0].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	suffix, ok := args[1].(string)
	if !ok {
		return nil, filters.ErrInvalidFilterParameters
	}

	f := &wrap{}
	if w.hex {
		var err error
		f.prefix, err = hex.DecodeString(prefix)
		if err != nil {
			return nil, err
		}
		f.suffix, err = hex.DecodeString(suffix)
		if err != nil {
			return nil, err
		}
	} else {
		f.prefix = []byte(prefix)
		f.suffix = []byte(suffix)
	}

	return f, nil
}

func (w *wrap) Request(ctx filters.FilterContext) {}

func (w *wrap) Response(ctx filters.FilterContext) {
	rsp := ctx.Response()

	if s := rsp.Header.Get("Content-Length"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			n += int64(len(w.prefix) + len(w.suffix))
			rsp.Header["Content-Length"] = []string{strconv.FormatInt(n, 10)}
		}
	}

	if rsp.ContentLength != -1 {
		rsp.ContentLength += int64(len(w.prefix) + len(w.suffix))
	}

	rsp.Body = &wrapReadCloser{
		Reader: io.MultiReader(
			bytes.NewReader(w.prefix),
			rsp.Body,
			bytes.NewReader(w.suffix),
		),
		Closer: rsp.Body,
	}
}

func (t *throttle) Name() string {
	switch t.typ {
	case latency:
		return filters.LatencyName
	case bandwidth:
		return filters.BandwidthName
	case chunks:
		return filters.ChunksName
	case backendLatency:
		return filters.BackendLatencyName
	case backendBandwidth:
		return filters.BackendBandwidthName
	case backendChunks:
		return filters.BackendChunksName
	default:
		panic("invalid throttle type")
	}
}

func parseDuration(v interface{}) (time.Duration, error) {
	var d time.Duration

	switch vt := v.(type) {
	case float64:
		d = time.Duration(vt) * time.Millisecond
	case string:
		var err error
		d, err = time.ParseDuration(vt)
		if err != nil {
			return 0, filters.ErrInvalidFilterParameters
		}
	}

	if d < 0 {
		return 0, filters.ErrInvalidFilterParameters
	}

	return d, nil
}

func parseLatencyArgs(args []interface{}) (int, time.Duration, error) {
	if len(args) != 1 {
		return 0, 0, filters.ErrInvalidFilterParameters
	}

	d, err := parseDuration(args[0])
	return 0, d, err
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

	d, err := parseDuration(args[1])
	return int(size), d, err
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
		time.Sleep(t.delay)
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
					delay -= time.Since(start)
				}

				time.Sleep(delay)
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

func (j *jitter) Name() string {
	switch j.typ {
	case normalRequestDistribution:
		return filters.NormalRequestLatencyName
	case uniformRequestDistribution:
		return filters.UniformRequestLatencyName
	case normalResponseDistribution:
		return filters.NormalResponseLatencyName
	case uniformResponseDistribution:
		return filters.UniformResponseLatencyName
	}
	return "unknown"
}

func (j *jitter) CreateFilter(args []interface{}) (filters.Filter, error) {
	var (
		mean  time.Duration
		delta time.Duration
		err   error
	)

	if len(args) != 2 {
		return nil, filters.ErrInvalidFilterParameters
	}
	if mean, err = parseDuration(args[0]); err != nil {
		return nil, fmt.Errorf("failed to parse duration mean %v: %w", args[0], err)
	}

	if delta, err = parseDuration(args[1]); err != nil {
		return nil, fmt.Errorf("failed to parse duration delta %v: %w", args[1], err)
	}

	return &jitter{
		typ:   j.typ,
		mean:  mean,
		delta: delta,
		sleep: time.Sleep,
	}, nil
}

func (j *jitter) Request(filters.FilterContext) {
	var r float64

	switch j.typ {
	case uniformRequestDistribution:
		r = 2*rand.Float64() - 1 // +/- sizing
	case normalRequestDistribution:
		r = rand.NormFloat64()
	default:
		return
	}
	f := r * float64(j.delta)
	j.sleep(j.mean + time.Duration(int64(f)))
}

func (j *jitter) Response(filters.FilterContext) {
	var r float64

	switch j.typ {
	case uniformResponseDistribution:
		r = 2*rand.Float64() - 1 // +/- sizing
	case normalResponseDistribution:
		r = rand.NormFloat64()
	default:
		return
	}
	f := r * float64(j.delta)
	j.sleep(j.mean + time.Duration(int64(f)))
}
