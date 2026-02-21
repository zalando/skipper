package diag_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/builtin"
	"github.com/zalando/skipper/filters/diag"
)

type histogram struct {
	buckets []time.Duration
	counts  []uint64
	total   uint64
}

func newHistogram(buckets ...time.Duration) *histogram {
	h := &histogram{
		buckets: buckets,
		counts:  make([]uint64, len(buckets)),
	}
	return h
}

func (h *histogram) add(v time.Duration) {
	i := 0
	for ; i < len(h.buckets)-1; i++ {
		if v >= h.buckets[i] && v < h.buckets[i+1] {
			break
		}
	}
	h.total++
	h.counts[i]++
}

func (h *histogram) normalizedCounts() []float64 {
	n := make([]float64, len(h.counts))
	if h.total > 0 {
		for i, c := range h.counts {
			n[i] = float64(c) / float64(h.total)
		}
	}
	return n
}

func TestHistogramInvalidArgs(t *testing.T) {
	registry := builtin.MakeRegistry()

	for _, def := range []string{
		`histogramRequestLatency()`,
		`histogramRequestLatency(1)`,
		`histogramRequestLatency("0ms", 1)`,
		`histogramRequestLatency("0ms", 1, "0ms")`,
		`histogramRequestLatency("0ms", 1, "1ms", 2)`,
		`histogramRequestLatency("0ms", 1, "1ms", 2, "-2ms")`,
		`histogramRequestLatency("0ms", 1, "1ms", 2, "1ms")`,
		`histogramRequestLatency("0ms", "1", "1ms")`,
		`histogramRequestLatency("0ms", 1, 1)`,
		`histogramRequestLatency("-1ms", 1, "1ms")`,
		`histogramRequestLatency("0ms", 0, "1ms")`,
		`histogramRequestLatency("0ms", 0, "1ms", 0, "2ms")`,
	} {
		check := func(t *testing.T) {
			ff, err := eskip.ParseFilters(def)
			require.NoError(t, err)
			require.Len(t, ff, 1)

			f := ff[0]

			spec := registry[f.Name]
			_, err = spec.CreateFilter(f.Args)

			assert.Error(t, err)
		}
		t.Run(def, check)
		t.Run(strings.ReplaceAll(def, filters.HistogramRequestLatencyName, filters.HistogramResponseLatencyName), check)
	}
}

func TestHistogramRequestLatency(t *testing.T) {
	spec := diag.NewHistogramRequestLatency()

	for _, tc := range []struct {
		def            string
		expectedCounts []float64
	}{
		{
			def:            `histogramRequestLatency("0ms", 1, "1ms", 3, "2ms")`,
			expectedCounts: []float64{0.25, 0.75},
		},
		{
			def:            `histogramRequestLatency("1ms", 1, "2ms", 2, "3ms", 3, "10ms", 4, "20ms")`,
			expectedCounts: []float64{0.1, 0.2, 0.3, 0.4},
		},
		{
			def:            `histogramRequestLatency("1ms", 1, "2ms", 0, "3ms", 0, "10ms", 1, "20ms")`,
			expectedCounts: []float64{0.5, 0, 0, 0.5},
		},
	} {
		t.Run(tc.def, func(t *testing.T) {
			def := eskip.MustParseFilters(tc.def)[0]

			f, err := spec.CreateFilter(def.Args)
			require.NoError(t, err)

			var histArgs []time.Duration
			for i := 0; i < len(def.Args); i += 2 {
				d, err := time.ParseDuration(def.Args[i].(string))
				require.NoError(t, err)
				histArgs = append(histArgs, d)
			}
			require.Equal(t, 1+len(tc.expectedCounts), len(histArgs))

			h := newHistogram(histArgs...)
			diag.SetSleep(f, h.add)

			// response ignored
			f.Response(nil)
			assert.Equal(t, uint64(0), h.total)

			const (
				nSamples = 20_000
				epsilon  = 0.1
			)
			for range nSamples {
				f.Request(nil)
			}

			assert.Equal(t, uint64(nSamples), h.total)
			assert.Equal(t, uint64(0), h.counts[len(tc.expectedCounts)], "last bucket count must be zero")

			for i, actual := range h.normalizedCounts()[:len(tc.expectedCounts)] {
				expected := tc.expectedCounts[i]
				if expected != 0 {
					diff := math.Abs(actual-expected) / expected
					if diff > epsilon {
						t.Errorf("bucket %d: expected %f, got %f, diff: %f", i, expected, actual, diff)
					}
				} else if actual != 0 {
					t.Errorf("bucket %d: expected %f, got %f", i, expected, actual)
				}
			}
		})
	}
}

func TestHistogramResponseLatency(t *testing.T) {
	spec := diag.NewHistogramResponseLatency()
	def := eskip.MustParseFilters(`histogramResponseLatency("0ms", 1, "1ms", 3, "2ms")`)[0]

	f, err := spec.CreateFilter(def.Args)
	require.NoError(t, err)

	total := 0
	diag.SetSleep(f, func(time.Duration) { total++ })

	// request ignored
	f.Request(nil)
	assert.Equal(t, 0, total)

	f.Response(nil)
	assert.Equal(t, 1, total)
}
