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
	mx   sync.Mutex
	rand *rand.Rand
	len  int64
}

type repeat struct {
	bytes []byte
	len   int64
}

type repeatReader struct {
	bytes  []byte
	offset int
}

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

// NewRepeat creates a filter specification whose filter instances can be used
// to respond to requests with a repeated text. It expects the text and
// the byte length of the response body to be generated as arguments.
// Eskip example:
//
// 	* -> repeatContent("x", 100) -> <shunt>;
//
func NewRepeat() filters.Spec { return &repeat{} }

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
// 	* -> chunks(1024, "120ms") -> "https://www.example.org";
//
func NewChunks() filters.Spec { return &throttle{typ: chunks} }

// NewBackendLatency is the equivalent of NewLatency but for outgoing backend
// requests. Eskip example:
//
// 	* -> backendLatency(120) -> "https://www.example.org";
//
func NewBackendLatency() filters.Spec { return &throttle{typ: backendLatency} }

// NewBackendBandwidth is the equivalent of NewBandwidth but for outgoing backend
// requests. Eskip example:
//
// 	* -> backendBandwidth(30) -> "https://www.example.org";
//
func NewBackendBandwidth() filters.Spec { return &throttle{typ: backendBandwidth} }

// NewBackendChunks is the equivalent of NewChunks but for outgoing backend
// requests. Eskip example:
//
// 	* -> backendChunks(1024, 120) -> "https://www.example.org";
//
func NewBackendChunks() filters.Spec { return &throttle{typ: backendChunks} }

// NewUniformRequestLatency creates a latency for requests with uniform
// distribution. Example delay around 1s with +/-120ms.
//
// 	* -> uniformRequestLatency("1s", "120ms") -> "https://www.example.org";
//
func NewUniformRequestLatency() filters.Spec { return &jitter{typ: uniformRequestDistribution} }

// NewNormalRequestLatency creates a latency for requests with normal
// distribution. Example delay around 1s with +/-120ms.
//
// 	* -> normalRequestLatency("1s", "120ms") -> "https://www.example.org";
//
func NewNormalRequestLatency() filters.Spec { return &jitter{typ: normalRequestDistribution} }

// NewUniformResponseLatency creates a latency for responses with uniform
// distribution. Example delay around 1s with +/-120ms.
//
// 	* -> uniformRequestLatency("1s", "120ms") -> "https://www.example.org";
//
func NewUniformResponseLatency() filters.Spec { return &jitter{typ: uniformResponseDistribution} }

// NewNormalResponseLatency creates a latency for responses with normal
// distribution. Example delay around 1s with +/-120ms.
//
// 	* -> normalRequestLatency("1s", "120ms") -> "https://www.example.org";
//
func NewNormalResponseLatency() filters.Spec { return &jitter{typ: normalResponseDistribution} }

func (r *random) Name() string { return filters.RandomContentName }

func (r *random) CreateFilter(args []interface{}) (filters.Filter, error) {
	a := filters.Args(args)
	len, err := a.Int64(), a.Err()
	if err != nil {
		return nil, err
	}
	if len < 0 {
		return nil, filters.ErrInvalidFilterParameters
	}
	return &random{rand: rand.New(rand.NewSource(time.Now().UnixNano())), len: len}, nil // #nosec
}

func (r *random) Read(p []byte) (int, error) {
	r.mx.Lock()
	defer r.mx.Unlock()
	for i := 0; i < len(p); i++ {
		p[i] = randomChars[r.rand.Intn(len(randomChars))]
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

func (r *repeat) Name() string { return filters.RepeatContentName }

func (r *repeat) CreateFilter(args []interface{}) (filters.Filter, error) {
	a := filters.Args(args)
	text, len, err := a.String(), a.Int64(), a.Err()
	if err != nil {
		return nil, err
	}
	if text == "" || len < 0 {
		return nil, filters.ErrInvalidFilterParameters
	}
	return &repeat{[]byte(text), len}, nil
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

func parseLatencyArgs(args []interface{}) (int, time.Duration, error) {
	a := filters.Args(args)
	return 0, a.DurationOrMilliseconds(), a.Err()
}

func parseBandwidthArgs(args []interface{}) (int, time.Duration, error) {
	a := filters.Args(args)
	kbps, err := a.Float64(), a.Err()

	if err != nil {
		return 0, 0, err
	}
	if kbps <= 0 {
		return 0, 0, filters.ErrInvalidFilterParameters
	}

	bpms := kbps2bpms(kbps)
	return defaultChunkSize, time.Duration(float64(defaultChunkSize)/bpms) * time.Millisecond, nil
}

func parseChunksArgs(args []interface{}) (int, time.Duration, error) {
	a := filters.Args(args)
	size, d, err := a.Int(), a.DurationOrMilliseconds(), a.Err()
	if err != nil {
		return 0, 0, err
	}
	if size <= 0 {
		return 0, 0, filters.ErrInvalidFilterParameters
	}
	return size, d, nil
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
	a := filters.Args(args)
	return &jitter{
		typ:   j.typ,
		mean:  a.DurationOrMilliseconds(),
		delta: a.DurationOrMilliseconds(),
	}, a.Err()
}

func (j *jitter) Request(filters.FilterContext) {
	var r float64

	switch j.typ {
	case uniformRequestDistribution:
		/* #nosec */
		r = 2*rand.Float64() - 1 // +/- sizing
	case normalRequestDistribution:
		r = rand.NormFloat64()
	default:
		return
	}
	f := r * float64(j.delta)
	time.Sleep(j.mean + time.Duration(int64(f)))
}

func (j *jitter) Response(filters.FilterContext) {
	var r float64

	switch j.typ {
	case uniformResponseDistribution:
		/* #nosec */
		r = 2*rand.Float64() - 1 // +/- sizing
	case normalResponseDistribution:
		r = rand.NormFloat64()
	default:
		return
	}
	f := r * float64(j.delta)
	time.Sleep(j.mean + time.Duration(int64(f)))
}
