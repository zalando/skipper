package diag

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/proxy/proxytest"
)

const (
	testDataChunks   = 16
	testDataLen      = testDataChunks * defaultChunkSize
	requestCheckName = "requestCheck"
	smallDelay       = 120
	highDelay        = 3 * smallDelay
	minEpsilon       = 30 * time.Millisecond
)

type testChunk struct {
	bytes    int
	duration time.Duration
}

type messageExp struct {
	header time.Duration
	chunks []testChunk
	kbps   float64
}

type requestCheck struct {
	check func(r *http.Request) bool
}

func (rc *requestCheck) Name() string                                         { return requestCheckName }
func (rc *requestCheck) CreateFilter(_ []interface{}) (filters.Filter, error) { return rc, nil }
func (rc *requestCheck) Response(_ filters.FilterContext)                     {}

func (rc *requestCheck) Request(ctx filters.FilterContext) {
	if !rc.check(ctx.Request()) {
		ctx.Serve(&http.Response{StatusCode: http.StatusBadRequest})
	}
}

func checkWithTolerance(start time.Time, expected time.Duration) bool {
	now := time.Now()

	epsilon := time.Duration(float64(expected) * 0.3)
	if epsilon < minEpsilon {
		epsilon = minEpsilon
	}

	lower := start.Add(expected - epsilon)
	higher := start.Add(expected + epsilon)
	return lower.Before(now) && higher.After(now)
}

func checkMessage(expect messageExp, start time.Time, body io.Reader) error {
	if expect.header > 0 && !checkWithTolerance(start, expect.header) {
		return fmt.Errorf("expected time to header, %v, %v, %v", start, time.Now(), expect.header)
	}

	totalRead := 0

	cstart := start
	for _, c := range expect.chunks {
		n := 0
		for n < c.bytes {
			ni, err := body.Read(make([]byte, c.bytes))
			if err != nil && err != io.EOF {
				return err
			}

			n += ni

			if err == io.EOF {
				break
			}
		}

		totalRead += n

		if !checkWithTolerance(cstart, c.duration) {
			return fmt.Errorf("expected chunk read time failed, %v, %v, %v", cstart, time.Now(), c.duration)
		}

		cstart = time.Now()
	}

	b, err := ioutil.ReadAll(body)
	if err != nil {
		return err
	}

	totalRead += len(b)

	if expect.kbps > 0 && !checkWithTolerance(start,
		time.Duration(float64(totalRead)/kbps2bpms(expect.kbps))*time.Millisecond) {
		return fmt.Errorf("expected bandwidth failed, %v, %v", start, time.Now())
	}

	return nil
}

func TestRandomArgs(t *testing.T) {
	for _, ti := range []struct {
		msg  string
		args []interface{}
		err  bool
	}{{
		"no args",
		nil,
		true,
	}, {
		"too many args",
		[]interface{}{float64(1), float64(2)},
		true,
	}, {
		"not a number",
		[]interface{}{"foo"},
		true,
	}, {
		"ok",
		[]interface{}{float64(42)},
		false,
	}} {
		_, err := NewRandom().CreateFilter(ti.args)
		switch {
		case err == nil && ti.err:
			t.Error(ti.msg, "failed to fail")
		case err != filters.ErrInvalidFilterParameters && ti.err:
			t.Error(ti.msg, "failed to fail with the right error")
		case err != nil && !ti.err:
			t.Error(ti.msg, err)
		}
	}
}

func TestRandom(t *testing.T) {
	for _, ti := range []struct {
		msg string
		len int
	}{{
		"zero bytes",
		0,
	}, {
		"small",
		defaultChunkSize / 2,
	}, {
		"large",
		defaultChunkSize*2 + defaultChunkSize/2,
	}} {
		func() {
			p := proxytest.New(filters.Registry{RandomName: &random{}}, &eskip.Route{
				Filters: []*eskip.Filter{{Name: RandomName, Args: []interface{}{float64(ti.len)}}},
				Shunt:   true})
			defer p.Close()

			rsp, err := http.Get(p.URL)
			if err != nil {
				t.Error(ti.msg, err)
				return
			}

			defer rsp.Body.Close()

			if rsp.StatusCode != http.StatusOK {
				t.Error(ti.msg, "request failed")
				return
			}

			b, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				t.Error(ti.msg, err)
				return
			}

			if len(b) != ti.len {
				t.Error(ti.msg, "invalid content length", len(b), ti.len)
				return
			}

			randBytes := []byte(randomChars)
			for _, bi := range b {
				found := false
				for _, rbi := range randBytes {
					if rbi == bi {
						found = true
						break
					}
				}

				if !found {
					t.Error(ti.msg, "invalid character")
					return
				}
			}
		}()
	}
}

func TestThrottleArgs(t *testing.T) {
	for _, ti := range []struct {
		msg  string
		spec func() filters.Spec
		args []interface{}
		err  bool
	}{{
		"latency, zero args",
		NewLatency,
		nil,
		true,
	}, {
		"latency, too many args",
		NewLatency,
		[]interface{}{float64(1), float64(2)},
		true,
	}, {
		"latency, not a number/duration string",
		NewLatency,
		[]interface{}{"foo"},
		true,
	}, {
		"latency, negative number",
		NewLatency,
		[]interface{}{float64(-1)},
		true,
	}, {
		"latency, negative duration string",
		NewLatency,
		[]interface{}{"-42us"},
		true,
	}, {
		"latency, ok number",
		NewLatency,
		[]interface{}{float64(1)},
		false,
	}, {
		"latency, ok duration string",
		NewLatency,
		[]interface{}{"42us"},
		false,
	}, {
		"backend latency, zero args",
		NewBackendLatency,
		nil,
		true,
	}, {
		"backend latency, too many args",
		NewBackendLatency,
		[]interface{}{float64(1), float64(2)},
		true,
	}, {
		"backend latency, not a number/duration string",
		NewBackendLatency,
		[]interface{}{"foo"},
		true,
	}, {
		"backend latency, negative number",
		NewBackendLatency,
		[]interface{}{float64(-1)},
		true,
	}, {
		"backend latency, negative duration string",
		NewBackendLatency,
		[]interface{}{"-42us"},
		true,
	}, {
		"backend latency, ok",
		NewBackendLatency,
		[]interface{}{float64(1)},
		false,
	}, {
		"bandwidth, zero args",
		NewBandwidth,
		nil,
		true,
	}, {
		"bandwidth, too many args",
		NewBandwidth,
		[]interface{}{float64(1), float64(2)},
		true,
	}, {
		"bandwidth, not a number",
		NewBandwidth,
		[]interface{}{"foo"},
		true,
	}, {
		"bandwidth, zero",
		NewBandwidth,
		[]interface{}{float64(0)},
		true,
	}, {
		"bandwidth, negative number",
		NewBandwidth,
		[]interface{}{float64(-1)},
		true,
	}, {
		"bandwidth, ok",
		NewBandwidth,
		[]interface{}{float64(1)},
		false,
	}, {
		"backend bandwidth, zero args",
		NewBackendBandwidth,
		nil,
		true,
	}, {
		"backend bandwidth, too many args",
		NewBackendBandwidth,
		[]interface{}{float64(1), float64(2)},
		true,
	}, {
		"backend bandwidth, not a number",
		NewBackendBandwidth,
		[]interface{}{"foo"},
		true,
	}, {
		"backend bandwidth, zero",
		NewBackendBandwidth,
		[]interface{}{float64(0)},
		true,
	}, {
		"backend bandwidth, negative number",
		NewBackendBandwidth,
		[]interface{}{float64(-1)},
		true,
	}, {
		"backend bandwidth, ok",
		NewBackendBandwidth,
		[]interface{}{float64(1)},
		false,
	}, {
		"chunks, too few args",
		NewChunks,
		[]interface{}{float64(1)},
		true,
	}, {
		"chunks, too many args",
		NewChunks,
		[]interface{}{float64(1), float64(2), float64(3)},
		true,
	}, {
		"chunks, size not a number",
		NewChunks,
		[]interface{}{"foo", float64(2)},
		true,
	}, {
		"chunks, delay not a number/duration string",
		NewChunks,
		[]interface{}{float64(1), "foo"},
		true,
	}, {
		"chunks, size zero",
		NewChunks,
		[]interface{}{float64(0), float64(2)},
		true,
	}, {
		"chunks, size negative",
		NewChunks,
		[]interface{}{float64(-1), float64(2)},
		true,
	}, {
		"chunks, delay negative",
		NewChunks,
		[]interface{}{float64(1), float64(-2)},
		true,
	}, {
		"chunks, delay negative duration string",
		NewChunks,
		[]interface{}{float64(1), "-42us"},
		true,
	}, {
		"chunks, ok",
		NewChunks,
		[]interface{}{float64(1), float64(2)},
		false,
	}, {
		"chunks, ok duration string",
		NewChunks,
		[]interface{}{float64(1), "42us"},
		false,
	}, {
		"backend chunks, too few args",
		NewBackendChunks,
		[]interface{}{float64(1)},
		true,
	}, {
		"backend chunks, too many args",
		NewBackendChunks,
		[]interface{}{float64(1), float64(2), float64(3)},
		true,
	}, {
		"backend chunks, size not a number",
		NewBackendChunks,
		[]interface{}{"foo", float64(2)},
		true,
	}, {
		"backend chunks, delay not a number/duration string",
		NewBackendChunks,
		[]interface{}{float64(1), "foo"},
		true,
	}, {
		"backend chunks, size zero",
		NewBackendChunks,
		[]interface{}{float64(0), float64(2)},
		true,
	}, {
		"backend chunks, size negative",
		NewBackendChunks,
		[]interface{}{float64(-1), float64(2)},
		true,
	}, {
		"backend chunks, delay negative",
		NewBackendChunks,
		[]interface{}{float64(1), float64(-2)},
		true,
	}, {
		"backend chunks, delay negative duration string",
		NewBackendChunks,
		[]interface{}{float64(1), "-42us"},
		true,
	}, {
		"backend chunks, ok",
		NewBackendChunks,
		[]interface{}{float64(1), float64(2)},
		false,
	}, {
		"backend chunks, ok duration string",
		NewBackendChunks,
		[]interface{}{float64(1), "42us"},
		false,
	}} {
		s := ti.spec()
		_, err := s.CreateFilter(ti.args)
		switch {
		case err == nil && ti.err:
			t.Error(ti.msg, "failed to fail")
		case err != filters.ErrInvalidFilterParameters && ti.err:
			t.Error(ti.msg, "failed to fail with the right error")
		case err != nil && !ti.err:
			t.Error(ti.msg, err)
		}
	}
}

func TestThrottle(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	rc := &requestCheck{}
	r := filters.Registry{
		requestCheckName: rc,
		RandomName:       &random{}}

	testServer := proxytest.New(r, &eskip.Route{
		Filters: []*eskip.Filter{
			{Name: requestCheckName, Args: nil},
			{Name: RandomName, Args: []interface{}{float64(testDataLen)}}},
		Shunt: true})
	defer testServer.Close()

	proxyFilters := filters.Registry{
		LatencyName:          NewLatency(),
		BandwidthName:        NewBandwidth(),
		ChunksName:           NewChunks(),
		BackendLatencyName:   NewBackendLatency(),
		BackendBandwidthName: NewBackendBandwidth(),
		BackendChunksName:    NewBackendChunks()}

	for _, ti := range []struct {
		msg           string
		filters       []*eskip.Filter
		clientExpect  messageExp
		backendExpect messageExp
	}{{
		msg:          "zero latency",
		filters:      []*eskip.Filter{{Name: LatencyName, Args: []interface{}{float64(0)}}},
		clientExpect: messageExp{header: time.Millisecond},
	}, {
		msg:          "latency",
		filters:      []*eskip.Filter{{Name: LatencyName, Args: []interface{}{float64(smallDelay)}}},
		clientExpect: messageExp{header: smallDelay * time.Millisecond},
	}, {
		msg:          "high latency",
		filters:      []*eskip.Filter{{Name: LatencyName, Args: []interface{}{float64(highDelay)}}},
		clientExpect: messageExp{header: highDelay * time.Millisecond},
	}, {
		msg:           "zero backend latency",
		filters:       []*eskip.Filter{{Name: BackendLatencyName, Args: []interface{}{float64(0)}}},
		backendExpect: messageExp{header: time.Millisecond},
	}, {
		msg:           "backend latency",
		filters:       []*eskip.Filter{{Name: BackendLatencyName, Args: []interface{}{float64(smallDelay)}}},
		backendExpect: messageExp{header: smallDelay * time.Millisecond},
	}, {
		msg:           "high backend latency",
		filters:       []*eskip.Filter{{Name: BackendLatencyName, Args: []interface{}{float64(highDelay)}}},
		backendExpect: messageExp{header: highDelay * time.Millisecond},
	}, {
		msg:          "bandwidth",
		filters:      []*eskip.Filter{{Name: BandwidthName, Args: []interface{}{float64(12)}}},
		clientExpect: messageExp{kbps: 12},
	}, {
		msg:     "very high bandwidth",
		filters: []*eskip.Filter{{Name: BandwidthName, Args: []interface{}{float64(12000000000)}}},
	}, {
		msg: "bandwidth, adjust",
		filters: []*eskip.Filter{{
			Name: BandwidthName,
			Args: []interface{}{float64(12)},
		}, {
			Name: BandwidthName,
			Args: []interface{}{float64(36)},
		}},
		clientExpect: messageExp{kbps: 12},
	}, {
		msg:           "backend bandwidth",
		filters:       []*eskip.Filter{{Name: BackendBandwidthName, Args: []interface{}{float64(12)}}},
		backendExpect: messageExp{kbps: 12},
	}, {
		msg:     "backend, very high bandwidth",
		filters: []*eskip.Filter{{Name: BackendBandwidthName, Args: []interface{}{float64(12000000000)}}},
	}, {
		msg: "backend bandwidth, adjust",
		filters: []*eskip.Filter{{
			Name: BackendBandwidthName,
			Args: []interface{}{float64(36)},
		}, {
			Name: BackendBandwidthName,
			Args: []interface{}{float64(12)},
		}},
		backendExpect: messageExp{kbps: 12},
	}, {
		msg:          "single chunk",
		filters:      []*eskip.Filter{{Name: ChunksName, Args: []interface{}{float64(2 * testDataLen), float64(smallDelay)}}},
		clientExpect: messageExp{chunks: []testChunk{{2 * testDataLen, time.Duration(smallDelay) * time.Millisecond}}},
	}, {
		msg:          "single chunk, long delay",
		filters:      []*eskip.Filter{{Name: ChunksName, Args: []interface{}{float64(2 * testDataLen), float64(highDelay)}}},
		clientExpect: messageExp{chunks: []testChunk{{2 * testDataLen, time.Duration(highDelay) * time.Millisecond}}},
	}, {
		msg:     "multiple chunks",
		filters: []*eskip.Filter{{Name: ChunksName, Args: []interface{}{float64(testDataLen/4 + testDataLen/8), float64(smallDelay)}}},
		clientExpect: messageExp{chunks: []testChunk{{
			testDataLen/4 + testDataLen/8, time.Duration(smallDelay) * time.Millisecond,
		}, {
			testDataLen/4 + testDataLen/8, time.Duration(smallDelay) * time.Millisecond,
		}, {
			testDataLen / 4, time.Duration(smallDelay) * time.Millisecond,
		}}},
	}, {
		msg:     "multiple chunks, long delay",
		filters: []*eskip.Filter{{Name: ChunksName, Args: []interface{}{float64(testDataLen/4 + testDataLen/8), float64(highDelay)}}},
		clientExpect: messageExp{chunks: []testChunk{{
			testDataLen/4 + testDataLen/8, time.Duration(highDelay) * time.Millisecond,
		}, {
			testDataLen/4 + testDataLen/8, time.Duration(highDelay) * time.Millisecond,
		}, {
			testDataLen / 4, time.Duration(highDelay) * time.Millisecond,
		}}},
	}, {
		msg:           "single chunk, backend",
		filters:       []*eskip.Filter{{Name: BackendChunksName, Args: []interface{}{float64(2 * testDataLen), float64(smallDelay)}}},
		backendExpect: messageExp{chunks: []testChunk{{2 * testDataLen, time.Duration(smallDelay) * time.Millisecond}}},
	}, {
		msg:           "single chunk, long delay, backend",
		filters:       []*eskip.Filter{{Name: BackendChunksName, Args: []interface{}{float64(2 * testDataLen), float64(highDelay)}}},
		backendExpect: messageExp{chunks: []testChunk{{2 * testDataLen, time.Duration(highDelay) * time.Millisecond}}},
	}, {
		msg:     "multiple chunks, backend",
		filters: []*eskip.Filter{{Name: BackendChunksName, Args: []interface{}{float64(testDataLen/4 + testDataLen/8), float64(smallDelay)}}},
		backendExpect: messageExp{chunks: []testChunk{{
			testDataLen/4 + testDataLen/8, time.Duration(smallDelay) * time.Millisecond,
		}, {
			testDataLen/4 + testDataLen/8, time.Duration(smallDelay) * time.Millisecond,
		}, {
			testDataLen / 4, time.Duration(smallDelay) * time.Millisecond,
		}}},
	}, {
		msg:     "multiple chunks, long delay, backend",
		filters: []*eskip.Filter{{Name: BackendChunksName, Args: []interface{}{float64(testDataLen/4 + testDataLen/8), float64(highDelay)}}},
		backendExpect: messageExp{chunks: []testChunk{{
			testDataLen/4 + testDataLen/8, time.Duration(highDelay) * time.Millisecond,
		}, {
			testDataLen/4 + testDataLen/8, time.Duration(highDelay) * time.Millisecond,
		}, {
			testDataLen / 4, time.Duration(highDelay) * time.Millisecond,
		}}},
	}} {
		func() {
			var requestStart time.Time

			rc.check = func(req *http.Request) bool {
				if err := checkMessage(ti.backendExpect, requestStart, req.Body); err != nil {
					t.Error(ti.msg, err)
					return false
				}

				return true
			}

			p := proxytest.New(proxyFilters, &eskip.Route{
				Filters: ti.filters,
				Backend: testServer.URL})
			defer p.Close()

			req, err := http.NewRequest("GET", p.URL,
				&io.LimitedReader{rand.New(rand.NewSource(0)), testDataLen})
			if err != nil {
				t.Error(ti.msg, err)
				return
			}

			requestStart = time.Now()
			rsp, err := (&http.Client{}).Do(req)
			if err != nil {
				t.Error(ti.msg, err)
				return
			}

			defer rsp.Body.Close()

			if rsp.StatusCode != http.StatusOK {
				if rsp.StatusCode != http.StatusBadRequest {
					t.Error(ti.msg, "request failed")
				}

				return
			}

			if err := checkMessage(ti.clientExpect, requestStart, rsp.Body); err != nil {
				t.Error(ti.msg, err)
			}

		}()
	}
}
