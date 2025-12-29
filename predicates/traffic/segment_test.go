package traffic_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/predicates/primitive"
	"github.com/zalando/skipper/predicates/tee"
	"github.com/zalando/skipper/predicates/traffic"
	"github.com/zalando/skipper/proxy/proxytest"
	"github.com/zalando/skipper/routing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrafficSegmentInvalidCreateArguments(t *testing.T) {
	spec := traffic.NewSegment()

	for _, def := range []string{
		`TrafficSegment()`,
		`TrafficSegment(1)`,
		`TrafficSegment(1, 0)`,
		`TrafficSegment(0, 1.1)`,
		`TrafficSegment(1, 2)`,
		`TrafficSegment(0, "1")`,
		`TrafficSegment("0", 1)`,
	} {
		t.Run(def, func(t *testing.T) {
			pp := eskip.MustParsePredicates(def)
			require.Len(t, pp, 1)

			_, err := spec.Create(pp[0].Args)
			assert.Error(t, err)
		})
	}
}

func requestWithR(r float64) *http.Request {
	req := &http.Request{}
	req = req.WithContext(routing.NewContext(req.Context()))

	_ = routing.FromContext(req.Context(), traffic.ExportRandomValue, func() float64 { return r })
	return req
}

// doN performs a number of requests and returns the number of responses for each status code and
// a total number of requests performed.
// Results use float64 type to simplify fractional comparisons.
func doN(t *testing.T, client *proxytest.TestClient, request func() *http.Request) (map[int]float64, float64) {
	const n = 10_000

	var mu sync.Mutex
	codes := make(map[int]float64)

	var g errgroup.Group
	g.SetLimit(runtime.NumCPU())

	for i := 0; i < n; i++ {
		g.Go(func() error {
			rsp, err := client.Do(request())
			if err != nil {
				return err
			}
			rsp.Body.Close()

			mu.Lock()
			codes[rsp.StatusCode]++
			mu.Unlock()

			return nil
		})
	}
	require.NoError(t, g.Wait())

	return codes, n
}

// assertEqualWithTolerance verifies that actual value is within predefined delta of the expected value.
func assertEqualWithTolerance(t *testing.T, expected, actual float64) {
	t.Helper()
	assert.InDelta(t, expected, actual, 0.06*expected)
}

func getN(t *testing.T, client *proxytest.TestClient, url string) (map[int]float64, float64) {
	return doN(t, client, func() *http.Request {
		req, err := http.NewRequest("GET", url, nil)
		require.NoError(t, err)
		return req
	})
}

func TestTrafficSegmentMatch(t *testing.T) {
	pp := eskip.MustParsePredicates(`TrafficSegment(0, 0.5)`)
	require.Len(t, pp, 1)

	spec := traffic.NewSegment()
	p, err := spec.Create(pp[0].Args)
	require.NoError(t, err)

	assert.True(t, p.Match(requestWithR(0.0)))
	assert.True(t, p.Match(requestWithR(0.1)))
	assert.True(t, p.Match(requestWithR(0.49)))

	assert.False(t, p.Match(requestWithR(0.5))) // upper interval boundary is excluded
	assert.False(t, p.Match(requestWithR(0.6)))
	assert.False(t, p.Match(requestWithR(1.0)))
}

func TestTrafficSegmentMinEqualsMax(t *testing.T) {
	for _, minMax := range []float64{
		0.0,
		1.0 / 10.0, // cannot be represented exactly as float64
		0.5,
		2.0 / 3.0, // cannot be represented exactly as float64
		1.0,
	} {
		t.Run(fmt.Sprintf("minMax=%v", minMax), func(t *testing.T) {
			spec := traffic.NewSegment()
			p, err := spec.Create([]any{minMax, minMax})
			require.NoError(t, err)

			assert.False(t, p.Match(requestWithR(minMax)))
		})
	}
}

func TestTrafficSegmentSpec(t *testing.T) {
	spec := traffic.NewSegment()

	assert.Equal(t, predicates.TrafficSegmentName, spec.Name())
	assert.Equal(t, -1, spec.Weight())
}

func TestTrafficSegmentSplit(t *testing.T) {
	p := proxytest.Config{
		RoutingOptions: routing.Options{
			FilterRegistry: builtin.MakeRegistry(),
			Predicates: []routing.PredicateSpec{
				// Use fixed random sequence to deflake the test,
				// see https://github.com/zalando/skipper/issues/2665
				traffic.WithRandFloat64(traffic.NewSegment(), newTestRandFloat64()),
			},
		},
		Routes: eskip.MustParse(`
			r50: Path("/test") && TrafficSegment(0.0, 0.5) -> status(200) -> inlineContent("") -> <shunt>;
			r30: Path("/test") && TrafficSegment(0.5, 0.8) -> status(201) -> inlineContent("") -> <shunt>;
			r20: Path("/test") && TrafficSegment(0.8, 1.0) -> status(202) -> inlineContent("") -> <shunt>;
		`),
	}.Create()
	defer p.Close()

	codes, n := getN(t, p.Client(), p.URL+"/test")

	t.Logf("Response codes: %v", codes)

	assertEqualWithTolerance(t, n*0.5, codes[200])
	assertEqualWithTolerance(t, n*0.3, codes[201])
	assertEqualWithTolerance(t, n*0.2, codes[202])
}

func TestTrafficSegmentRouteWeight(t *testing.T) {
	p := proxytest.Config{
		RoutingOptions: routing.Options{
			FilterRegistry: builtin.MakeRegistry(),
			Predicates: []routing.PredicateSpec{
				traffic.NewSegment(),
			},
		},
		Routes: eskip.MustParse(`
			segment90: Path("/test") && TrafficSegment(0.0, 0.9) -> status(200) -> inlineContent("") -> <shunt>;
			segment10: Path("/test") && TrafficSegment(0.9, 1.0) -> status(200) -> inlineContent("") -> <shunt>;
			cookie:    Path("/test") && Header("X-Foo", "bar")   -> status(201) -> inlineContent("") -> <shunt>;
		`),
	}.Create()
	defer p.Close()

	codes, n := getN(t, p.Client(), p.URL+"/test")
	assert.Equal(t, n, codes[200])

	codes, n = doN(t, p.Client(), func() *http.Request {
		req, _ := http.NewRequest("GET", p.URL+"/test", nil)
		req.Header.Set("X-Foo", "bar")
		return req
	})
	assert.Equal(t, n, codes[201])
}

func TestTrafficSegmentTeeLoopback(t *testing.T) {
	loopRequestsPtr := new(int32)
	loopBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(loopRequestsPtr, 1)
	}))
	defer loopBackend.Close()

	p := proxytest.Config{
		RoutingOptions: routing.Options{
			FilterRegistry: builtin.MakeRegistry(),
			Predicates: []routing.PredicateSpec{
				traffic.NewSegment(),
				tee.New(),
				primitive.NewTrue(),
			},
		},
		Routes: eskip.MustParse(fmt.Sprintf(`
			r0: * -> status(200) -> inlineContent("") -> <shunt>;
			r1: Path("/test") && TrafficSegment(0.0, 0.5) -> teeLoopback("a-loop") -> status(201) -> inlineContent("") -> <shunt>;
			r2: Path("/test") && Tee("a-loop") && True() -> "%s";
		`, loopBackend.URL)),
	}.Create()
	defer p.Close()

	codes, n := getN(t, p.Client(), p.URL+"/test")

	// wait for loopback requests to complete
	time.Sleep(100 * time.Millisecond)

	loopRequests := float64(atomic.LoadInt32(loopRequestsPtr))

	t.Logf("Response codes: %v, loopRequests: %f", codes, loopRequests)

	assertEqualWithTolerance(t, n*0.5, codes[200])
	assertEqualWithTolerance(t, n*0.5, codes[201])
	assert.Equal(t, codes[201], loopRequests)
}

func TestTrafficSegmentLoopbackBackend(t *testing.T) {
	p := proxytest.Config{
		RoutingOptions: routing.Options{
			FilterRegistry: builtin.MakeRegistry(),
			Predicates: []routing.PredicateSpec{
				traffic.NewSegment(),
				tee.New(),
				primitive.NewTrue(),
			},
		},
		Routes: eskip.MustParse(`
			r0: * -> status(200) -> inlineContent("") -> <shunt>;
			r1: Path("/test") && TrafficSegment(0.0, 0.5) -> setPath("a-loop") -> <loopback>;
			r2: Path("/a-loop") -> status(201) -> inlineContent("") -> <shunt>;
		`),
	}.Create()
	defer p.Close()

	codes, n := getN(t, p.Client(), p.URL+"/test")

	t.Logf("Response codes: %v", codes)

	assertEqualWithTolerance(t, n*0.5, codes[200])
	assertEqualWithTolerance(t, n*0.5, codes[201])
}
