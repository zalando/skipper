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

const minChunkSize = 512

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

func NewRandom() filters.Spec           { return &random{} }
func NewLatency() filters.Spec          { return &throttle{typ: latency} }
func NewBandwidth() filters.Spec        { return &throttle{typ: bandwidth} }
func NewChunks() filters.Spec           { return &throttle{typ: chunks} }
func NewBackendLatency() filters.Spec   { return &throttle{typ: backendLatency} }
func NewBackendBandwidth() filters.Spec { return &throttle{typ: backendBandwidth} }
func NewBackendChunks() filters.Spec    { return &throttle{typ: backendChunks} }

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
		l := minChunkSize
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
	return minChunkSize, time.Duration(float64(minChunkSize)/bpms) * time.Millisecond, nil
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

		b := make([]byte, minChunkSize)
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

				ni, err = in.Read(b)
				if err == io.EOF {
					eof = true
					err = nil
				}

				if err != nil {
					break
				}

				ni, err = w.Write(b[:ni])
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
