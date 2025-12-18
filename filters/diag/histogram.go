package diag

import (
	"math/rand/v2"
	"time"

	"github.com/zalando/skipper/filters"
)

type (
	histSpec struct {
		typ string
	}

	histFilter struct {
		response bool
		sleep    func(time.Duration)

		weights    []float64
		boundaries []time.Duration
	}
)

// NewHistogramRequestLatency creates filters that add latency to requests according to the histogram distribution.
// It expects a list of interleaved duration strings and numbers that defines a histogram.
// Duration strings define boundaries of consecutive buckets and numbers define bucket weights.
// The filter randomly selects a bucket with probability equal to its weight divided by the sum of all bucket weights
// (which must be non-zero) and then sleeps for a random duration in between bucket boundaries.
// Eskip example:
//
//	r: * -> histogramRequestLatency("0ms", 50, "5ms", 0, "10ms", 30, "15ms", 20, "20ms") -> "https://www.example.org";
//
// The example above adds a latency
// * between 0ms and 5ms to 50% of the requests
// * between 5ms and 10ms to 0% of the requests
// * between 10ms and 15ms to 30% of the requests
// * and between 15ms and 20ms to 20% of the requests.
func NewHistogramRequestLatency() filters.Spec {
	return &histSpec{typ: filters.HistogramRequestLatencyName}
}

// NewHistogramResponseLatency creates filters that add latency to responses according to the histogram distribution, similar to NewHistogramRequestLatency.
func NewHistogramResponseLatency() filters.Spec {
	return &histSpec{typ: filters.HistogramResponseLatencyName}
}

func (s *histSpec) Name() string {
	return s.typ
}

func (s *histSpec) CreateFilter(args []any) (filters.Filter, error) {
	if len(args) < 3 || len(args)%2 != 1 {
		return nil, filters.ErrInvalidFilterParameters
	}

	f := &histFilter{
		response: s.typ == filters.HistogramResponseLatencyName,
		sleep:    time.Sleep,
	}

	sum := 0.0
	for i, arg := range args {
		if i%2 == 0 {
			ds, ok := arg.(string)
			if !ok {
				return nil, filters.ErrInvalidFilterParameters
			}
			d, err := time.ParseDuration(ds)
			if err != nil || d < 0 {
				return nil, filters.ErrInvalidFilterParameters
			}
			if len(f.boundaries) > 0 && d <= f.boundaries[len(f.boundaries)-1] {
				return nil, filters.ErrInvalidFilterParameters
			}
			f.boundaries = append(f.boundaries, d)
		} else {
			weight, ok := arg.(float64)
			if !ok || weight < 0 {
				return nil, filters.ErrInvalidFilterParameters
			}
			f.weights = append(f.weights, weight)
			sum += weight
		}
	}

	if sum == 0 {
		return nil, filters.ErrInvalidFilterParameters
	}

	for i := range f.weights {
		f.weights[i] /= sum
	}

	return f, nil
}

func (f *histFilter) Request(filters.FilterContext) {
	if !f.response {
		f.sleep(f.sample())
	}
}

func (f *histFilter) Response(filters.FilterContext) {
	if f.response {
		f.sleep(f.sample())
	}
}

func (f *histFilter) sample() time.Duration {
	r := rand.Float64() // #nosec
	i, w, sum := 0, 0.0, 0.0
	for i, w = range f.weights {
		sum += w
		if sum > r {
			break
		}
	}
	// len(f.boundaries) = len(f.weights) + 1
	min := f.boundaries[i]
	max := f.boundaries[i+1]

	return min + time.Duration(rand.Int64N(int64(max-min))) // #nosec
}
