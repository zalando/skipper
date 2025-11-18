package diag

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sort"
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

	b, err := io.ReadAll(body)
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
			p := proxytest.New(filters.Registry{filters.RandomContentName: &random{}}, &eskip.Route{
				Filters: []*eskip.Filter{{Name: filters.RandomContentName, Args: []interface{}{float64(ti.len)}}},
				Shunt:   true})
			defer p.Close()

			req, err := http.NewRequest("GET", p.URL, nil)
			if err != nil {
				t.Error(ti.msg, err)
				return
			}

			req.Close = true

			rsp, err := (&http.Client{}).Do(req)
			if err != nil {
				t.Error(ti.msg, err)
				return
			}

			defer rsp.Body.Close()

			if rsp.StatusCode != http.StatusOK {
				t.Error(ti.msg, "request failed")
				return
			}

			b, err := io.ReadAll(rsp.Body)
			if err != nil {
				t.Error(ti.msg, err)
				return
			}

			if len(b) != ti.len {
				t.Error(ti.msg, "invalid content length", len(b), ti.len)
				return
			}

			for _, bi := range b {
				found := false
				for _, rbi := range randomChars {
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

func TestRepeatReader(t *testing.T) {
	r := &repeatReader{[]byte("0123456789"), 0}

	checkRead(t, r, 5, "01234")
	checkRead(t, r, 5, "56789")
	checkRead(t, r, 3, "012")
	checkRead(t, r, 3, "345")
	checkRead(t, r, 3, "678")
	checkRead(t, r, 3, "901")
	checkRead(t, r, 10, "2345678901")
	checkRead(t, r, 8, "23456789")
	checkRead(t, r, 15, "012345678901234")
	checkRead(t, r, 25, "5678901234567890123456789")
	checkRead(t, r, 1, "0")
	checkRead(t, r, 2, "12")
	checkRead(t, r, 3, "345")
	checkRead(t, r, 4, "6789")
	checkRead(t, r, 5, "01234")
	checkRead(t, r, 6, "567890")
	checkRead(t, r, 7, "1234567")
	checkRead(t, r, 8, "89012345")
	checkRead(t, r, 9, "678901234")
	checkRead(t, r, 10, "5678901234")
	checkRead(t, r, 11, "56789012345")
	checkRead(t, r, 12, "678901234567")
}

func checkRead(t *testing.T, r io.Reader, n int, expected string) {
	b := make([]byte, n)
	m, err := r.Read(b)
	if err != nil {
		t.Error(err)
		return
	}
	if m != n {
		t.Errorf("expected to read %d bytes, got %d", n, m)
		return
	}
	s := string(b)
	if s != expected {
		t.Errorf("expected %s, got %s", expected, s)
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
		t.Run(ti.msg, func(t *testing.T) {
			t.Parallel()
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
		})
	}
}

func TestThrottle(t *testing.T) {
	for _, ti := range []struct {
		msg           string
		filters       []*eskip.Filter
		clientExpect  messageExp
		backendExpect messageExp
	}{{
		msg:          "zero latency",
		filters:      []*eskip.Filter{{Name: filters.LatencyName, Args: []interface{}{float64(0)}}},
		clientExpect: messageExp{header: time.Millisecond},
	}, {
		msg:          "latency",
		filters:      []*eskip.Filter{{Name: filters.LatencyName, Args: []interface{}{float64(smallDelay)}}},
		clientExpect: messageExp{header: smallDelay * time.Millisecond},
	}, {
		msg:          "high latency",
		filters:      []*eskip.Filter{{Name: filters.LatencyName, Args: []interface{}{float64(highDelay)}}},
		clientExpect: messageExp{header: highDelay * time.Millisecond},
	}, {
		msg:           "zero backend latency",
		filters:       []*eskip.Filter{{Name: filters.BackendLatencyName, Args: []interface{}{float64(0)}}},
		backendExpect: messageExp{header: time.Millisecond},
	}, {
		msg:           "backend latency",
		filters:       []*eskip.Filter{{Name: filters.BackendLatencyName, Args: []interface{}{float64(smallDelay)}}},
		backendExpect: messageExp{header: smallDelay * time.Millisecond},
	}, {
		msg:           "high backend latency",
		filters:       []*eskip.Filter{{Name: filters.BackendLatencyName, Args: []interface{}{float64(highDelay)}}},
		backendExpect: messageExp{header: highDelay * time.Millisecond},
	}, {
		msg:          "bandwidth",
		filters:      []*eskip.Filter{{Name: filters.BandwidthName, Args: []interface{}{float64(12)}}},
		clientExpect: messageExp{kbps: 12},
	}, {
		msg:     "very high bandwidth",
		filters: []*eskip.Filter{{Name: filters.BandwidthName, Args: []interface{}{float64(12000000000)}}},
	}, {
		msg: "bandwidth, adjust",
		filters: []*eskip.Filter{{
			Name: filters.BandwidthName,
			Args: []interface{}{float64(12)},
		}, {
			Name: filters.BandwidthName,
			Args: []interface{}{float64(36)},
		}},
		clientExpect: messageExp{kbps: 12},
	}, {
		msg:           "backend bandwidth",
		filters:       []*eskip.Filter{{Name: filters.BackendBandwidthName, Args: []interface{}{float64(12)}}},
		backendExpect: messageExp{kbps: 12},
	}, {
		msg:     "backend, very high bandwidth",
		filters: []*eskip.Filter{{Name: filters.BackendBandwidthName, Args: []interface{}{float64(12000000000)}}},
	}, {
		msg: "backend bandwidth, adjust",
		filters: []*eskip.Filter{{
			Name: filters.BackendBandwidthName,
			Args: []interface{}{float64(36)},
		}, {
			Name: filters.BackendBandwidthName,
			Args: []interface{}{float64(12)},
		}},
		backendExpect: messageExp{kbps: 12},
	}, {
		msg:          "single chunk",
		filters:      []*eskip.Filter{{Name: filters.ChunksName, Args: []interface{}{float64(2 * testDataLen), float64(smallDelay)}}},
		clientExpect: messageExp{chunks: []testChunk{{2 * testDataLen, time.Duration(smallDelay) * time.Millisecond}}},
	}, {
		msg:          "single chunk, long delay",
		filters:      []*eskip.Filter{{Name: filters.ChunksName, Args: []interface{}{float64(2 * testDataLen), float64(highDelay)}}},
		clientExpect: messageExp{chunks: []testChunk{{2 * testDataLen, time.Duration(highDelay) * time.Millisecond}}},
	}, {
		msg:     "multiple chunks",
		filters: []*eskip.Filter{{Name: filters.ChunksName, Args: []interface{}{float64(testDataLen/4 + testDataLen/8), float64(smallDelay)}}},
		clientExpect: messageExp{chunks: []testChunk{{
			testDataLen/4 + testDataLen/8, time.Duration(smallDelay) * time.Millisecond,
		}, {
			testDataLen/4 + testDataLen/8, time.Duration(smallDelay) * time.Millisecond,
		}, {
			testDataLen / 4, time.Duration(smallDelay) * time.Millisecond,
		}}},
	}, {
		msg:     "multiple chunks, long delay",
		filters: []*eskip.Filter{{Name: filters.ChunksName, Args: []interface{}{float64(testDataLen/4 + testDataLen/8), float64(highDelay)}}},
		clientExpect: messageExp{chunks: []testChunk{{
			testDataLen/4 + testDataLen/8, time.Duration(highDelay) * time.Millisecond,
		}, {
			testDataLen/4 + testDataLen/8, time.Duration(highDelay) * time.Millisecond,
		}, {
			testDataLen / 4, time.Duration(highDelay) * time.Millisecond,
		}}},
	}, {
		msg:           "single chunk, backend",
		filters:       []*eskip.Filter{{Name: filters.BackendChunksName, Args: []interface{}{float64(2 * testDataLen), float64(smallDelay)}}},
		backendExpect: messageExp{chunks: []testChunk{{2 * testDataLen, time.Duration(smallDelay) * time.Millisecond}}},
	}, {
		msg:           "single chunk, long delay, backend",
		filters:       []*eskip.Filter{{Name: filters.BackendChunksName, Args: []interface{}{float64(2 * testDataLen), float64(highDelay)}}},
		backendExpect: messageExp{chunks: []testChunk{{2 * testDataLen, time.Duration(highDelay) * time.Millisecond}}},
	}, {
		msg:     "multiple chunks, backend",
		filters: []*eskip.Filter{{Name: filters.BackendChunksName, Args: []interface{}{float64(testDataLen/4 + testDataLen/8), float64(smallDelay)}}},
		backendExpect: messageExp{chunks: []testChunk{{
			testDataLen/4 + testDataLen/8, time.Duration(smallDelay) * time.Millisecond,
		}, {
			testDataLen/4 + testDataLen/8, time.Duration(smallDelay) * time.Millisecond,
		}, {
			testDataLen / 4, time.Duration(smallDelay) * time.Millisecond,
		}}},
	}, {
		msg:     "multiple chunks, long delay, backend",
		filters: []*eskip.Filter{{Name: filters.BackendChunksName, Args: []interface{}{float64(testDataLen/4 + testDataLen/8), float64(highDelay)}}},
		backendExpect: messageExp{chunks: []testChunk{{
			testDataLen/4 + testDataLen/8, time.Duration(highDelay) * time.Millisecond,
		}, {
			testDataLen/4 + testDataLen/8, time.Duration(highDelay) * time.Millisecond,
		}, {
			testDataLen / 4, time.Duration(highDelay) * time.Millisecond,
		}}},
	}} {
		t.Run(ti.msg, func(t *testing.T) {
			t.Parallel()
			rc := &requestCheck{}
			r := filters.Registry{
				requestCheckName:          rc,
				filters.RandomContentName: &random{}}

			testServer := proxytest.New(r, &eskip.Route{
				Filters: []*eskip.Filter{
					{Name: requestCheckName, Args: nil},
					{Name: filters.RandomContentName, Args: []interface{}{float64(testDataLen)}}},
				Shunt: true})
			defer testServer.Close()

			proxyFilters := filters.Registry{
				filters.LatencyName:          NewLatency(),
				filters.BandwidthName:        NewBandwidth(),
				filters.ChunksName:           NewChunks(),
				filters.BackendLatencyName:   NewBackendLatency(),
				filters.BackendBandwidthName: NewBackendBandwidth(),
				filters.BackendChunksName:    NewBackendChunks()}
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
				&io.LimitedReader{R: rand.New(rand.NewSource(0)), N: testDataLen})
			if err != nil {
				t.Error(ti.msg, err)
				return
			}

			req.Close = true

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

		})
	}
}

func TestLatencyArgs(t *testing.T) {
	for _, ti := range []struct {
		msg  string
		spec func() filters.Spec
		args []interface{}
		err  bool
	}{
		{
			"no uniform request latency args",
			NewUniformRequestLatency,
			nil,
			true,
		}, {
			"uniform request latency, false number of args 3",
			NewUniformRequestLatency,
			[]interface{}{"1s", "1s", 0.4},
			true,
		}, {
			"uniform request latency, false number of args 4",
			NewUniformRequestLatency,
			[]interface{}{"1s", "1s", 0.5, 0.1},
			true,
		}, {
			"uniform request latency, ok duration string",
			NewUniformRequestLatency,
			[]interface{}{"1s", "120ms"},
			false,
		}, {
			"normal request latency ok",
			NewNormalRequestLatency,
			[]interface{}{"1s", "1s"},
			false,
		}, {
			"response uniform latency, ok duration string",
			NewUniformResponseLatency,
			[]interface{}{"1s", "120ms"},
			false,
		}, {
			"response normal latency ok",
			NewNormalResponseLatency,
			[]interface{}{"1s", "1s"},
			false,
		}} {

		t.Run(ti.msg, func(t *testing.T) {
			s := ti.spec()
			_, err := s.CreateFilter(ti.args)
			switch {
			case err == nil && ti.err:
				t.Errorf("failed to fail: %v", err)
			case err != filters.ErrInvalidFilterParameters && ti.err:
				t.Errorf("failed to fail with the right error: %v", err)
			case err != nil && !ti.err:
				t.Error(ti.msg, err)
			}
		})
	}
}

// test distribution with:
// % bin/skipper -inline-routes='* -> uniformRequestLatency("100ms", "20ms") -> status(204) -> <shunt>' -access-log-disabled
// % echo "GET http://localhost:9090/test" | vegeta attack -rate 500/s -duration 1m | vegeta report -type hist[0,80ms,90ms,100ms,110ms,120ms]
func TestRequestLatency(t *testing.T) {
	for _, ti := range []struct {
		msg                     string
		spec                    func() filters.Spec
		args                    []interface{}
		p10, p25, p50, p75, p90 time.Duration
		epsilon                 time.Duration
	}{
		{
			msg:     "test uniform latency",
			spec:    NewUniformRequestLatency,
			args:    []interface{}{"10ms", "5ms"},
			p10:     6 * time.Millisecond,
			p25:     8 * time.Millisecond,
			p50:     11 * time.Millisecond,
			p75:     13 * time.Millisecond,
			p90:     14 * time.Millisecond,
			epsilon: 2 * time.Millisecond,
		},
		{
			msg:     "test normal latency",
			spec:    NewNormalRequestLatency,
			args:    []interface{}{"10ms", "5ms"},
			p10:     4 * time.Millisecond,
			p25:     7 * time.Millisecond,
			p50:     11 * time.Millisecond,
			p75:     14 * time.Millisecond,
			p90:     17 * time.Millisecond,
			epsilon: 2 * time.Millisecond,
		}} {

		t.Run(ti.msg, func(t *testing.T) {
			s := ti.spec()
			f, err := s.CreateFilter(ti.args)
			if err != nil {
				t.Errorf("Failed to create filter for args '%v': %v", ti.args, err)
			}

			N := 1000

			res := make([]time.Duration, 0, N)
			SetSleep(f, func(d time.Duration) { res = append(res, d) })

			for i := 0; i < N; i++ {
				f.Request(nil)
			}

			sort.Slice(res, func(i, j int) bool {
				return res[i] < res[j]
			})
			normalN := N / 100
			p10 := res[10*normalN]
			if ti.p10 < p10-ti.epsilon || ti.p10 > p10+ti.epsilon {
				t.Errorf("p10 not in range want p10=%s with epsilon=%s, got: %s", ti.p10, ti.epsilon, p10)
			}
			p25 := res[25*normalN]
			if ti.p25 < p25-ti.epsilon || ti.p25 > p25+ti.epsilon {
				t.Errorf("p25 not in range want p25=%s with epsilon=%s, got: %s", ti.p25, ti.epsilon, p25)
			}
			p50 := res[50*normalN]
			if ti.p50 < p50-ti.epsilon || ti.p50 > p50+ti.epsilon {
				t.Errorf("p50 not in range want p50=%s with epsilon=%s, got: %s", ti.p50, ti.epsilon, p50)
			}
			p75 := res[75*normalN]
			if ti.p75 < p75-ti.epsilon || ti.p75 > p75+ti.epsilon {
				t.Errorf("p75 not in range want p75=%s with epsilon=%s, got: %s", ti.p75, ti.epsilon, p75)
			}
			p90 := res[90*normalN]
			if ti.p90 < p90-ti.epsilon || ti.p90 > p90+ti.epsilon {
				t.Errorf("p90 not in range want p90=%s with epsilon=%s, got: %s", ti.p90, ti.epsilon, p90)
			}

		})
	}
}

// test distribution with:
// % bin/skipper -inline-routes='* -> uniformRequestLatency("100ms", "20ms") -> status(204) -> <shunt>' -access-log-disabled
// % echo "GET http://localhost:9090/test" | vegeta attack -rate 500/s -duration 1m | vegeta report -type hist[0,80ms,90ms,100ms,110ms,120ms]
func TestResponseLatency(t *testing.T) {
	for _, ti := range []struct {
		msg                     string
		spec                    func() filters.Spec
		args                    []interface{}
		p10, p25, p50, p75, p90 time.Duration
		epsilon                 time.Duration
	}{
		{
			msg:     "test response uniform latency",
			spec:    NewUniformResponseLatency,
			args:    []interface{}{"10ms", "5ms"},
			p10:     7 * time.Millisecond,
			p25:     8 * time.Millisecond,
			p50:     11 * time.Millisecond,
			p75:     13 * time.Millisecond,
			p90:     14 * time.Millisecond,
			epsilon: 2 * time.Millisecond,
		},
		{
			msg:     "test response normal latency",
			spec:    NewNormalResponseLatency,
			args:    []interface{}{"10ms", "5ms"},
			p10:     4 * time.Millisecond,
			p25:     7 * time.Millisecond,
			p50:     11 * time.Millisecond,
			p75:     14 * time.Millisecond,
			p90:     17 * time.Millisecond,
			epsilon: 2 * time.Millisecond,
		}} {

		t.Run(ti.msg, func(t *testing.T) {
			s := ti.spec()
			f, err := s.CreateFilter(ti.args)
			if err != nil {
				t.Errorf("Failed to create filter for args '%v': %v", ti.args, err)
			}

			N := 1000

			res := make([]time.Duration, 0, N)
			SetSleep(f, func(d time.Duration) { res = append(res, d) })

			for i := 0; i < N; i++ {
				f.Response(nil)
			}

			sort.Slice(res, func(i, j int) bool {
				return res[i] < res[j]
			})
			normalN := N / 100
			p10 := res[10*normalN]
			if ti.p10 < p10-ti.epsilon || ti.p10 > p10+ti.epsilon {
				t.Errorf("p10 not in range want p10=%s with epsilon=%s, got: %s", ti.p10, ti.epsilon, p10)
			}
			p25 := res[25*normalN]
			if ti.p25 < p25-ti.epsilon || ti.p25 > p25+ti.epsilon {
				t.Errorf("p25 not in range want p25=%s with epsilon=%s, got: %s", ti.p25, ti.epsilon, p25)
			}
			p50 := res[50*normalN]
			if ti.p50 < p50-ti.epsilon || ti.p50 > p50+ti.epsilon {
				t.Errorf("p50 not in range want p50=%s with epsilon=%s, got: %s", ti.p50, ti.epsilon, p50)
			}
			p75 := res[75*normalN]
			if ti.p75 < p75-ti.epsilon || ti.p75 > p75+ti.epsilon {
				t.Errorf("p75 not in range want p75=%s with epsilon=%s, got: %s", ti.p75, ti.epsilon, p75)
			}
			p90 := res[90*normalN]
			if ti.p90 < p90-ti.epsilon || ti.p90 > p90+ti.epsilon {
				t.Errorf("p90 not in range want p90=%s with epsilon=%s, got: %s", ti.p90, ti.epsilon, p90)
			}

		})
	}
}
