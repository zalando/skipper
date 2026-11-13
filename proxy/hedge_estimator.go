package proxy

import (
	"math/bits"
	"sync"
	"sync/atomic"
	"time"
)

const (
	latencyBuckets      = 304
	latencyMaxMicros    = 1<<40 - 1
	latencyWindow       = 30 * time.Second
	latencyRefresh      = time.Second
	latencyMinSamples   = 20
	latencyWarmupSample = 100
)

func latencyBucket(micros uint64) int {
	if micros > latencyMaxMicros {
		micros = latencyMaxMicros
	}
	if micros < 16 {
		return int(micros)
	}
	e := bits.Len64(micros) - 1
	m := (micros >> (e - 3)) & 7
	return (e-2)*8 + int(m)
}

func latencyBucketUpper(idx int) time.Duration {
	if idx < 16 {
		return time.Duration(idx+1) * time.Microsecond
	}
	e := idx/8 + 2
	m := uint64(idx % 8)
	upper := (8 + m + 1) << (e - 3)
	return time.Duration(upper) * time.Microsecond
}

type latencyWindowCounts struct {
	id     atomic.Int64
	total  atomic.Int64
	counts [latencyBuckets]atomic.Uint32
}

type latencyEstimator struct {
	quantile float64

	mu  sync.Mutex
	win [2]latencyWindowCounts

	cached   atomic.Int64
	cachedAt atomic.Int64
}

func newLatencyEstimator(quantile float64) *latencyEstimator {
	return &latencyEstimator{quantile: quantile}
}

func windowID(now time.Time) int64 { return now.UnixNano() / int64(latencyWindow) }

func (e *latencyEstimator) add(now time.Time, d time.Duration) {
	if d < 0 {
		d = 0
	}
	id := windowID(now)
	w := &e.win[id&1]
	if w.id.Load() != id {
		e.rotate(w, id)
	}
	w.counts[latencyBucket(uint64(d/time.Microsecond))].Add(1)
	w.total.Add(1)
}

func (e *latencyEstimator) rotate(w *latencyWindowCounts, id int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if w.id.Load() == id {
		return
	}
	for i := range w.counts {
		w.counts[i].Store(0)
	}
	w.total.Store(0)
	w.id.Store(id)
}

func (e *latencyEstimator) reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.win {
		w := &e.win[i]
		for j := range w.counts {
			w.counts[j].Store(0)
		}
		w.total.Store(0)
		w.id.Store(0)
	}
	e.cached.Store(0)
	e.cachedAt.Store(0)
}

func (e *latencyEstimator) live(now time.Time) (cur, prev *latencyWindowCounts) {
	id := windowID(now)
	if w := &e.win[id&1]; w.id.Load() == id {
		cur = w
	}
	if w := &e.win[(id-1)&1]; w.id.Load() == id-1 {
		prev = w
	}
	return cur, prev
}

func quantileOf(q float64, ws ...*latencyWindowCounts) (time.Duration, int64) {
	var total int64
	for _, w := range ws {
		if w != nil {
			total += w.total.Load()
		}
	}
	if total == 0 {
		return 0, 0
	}

	rank := int64(q*float64(total)) + 1
	if rank > total {
		rank = total
	}

	var seen int64
	for i := 0; i < latencyBuckets; i++ {
		for _, w := range ws {
			if w != nil {
				seen += int64(w.counts[i].Load())
			}
		}
		if seen >= rank {
			return latencyBucketUpper(i), total
		}
	}
	return latencyBucketUpper(latencyBuckets - 1), total
}

// percentile returns the configured quantile of the last one to two windows
// and the number of samples behind it. The value is refreshed at most once
// per second.
func (e *latencyEstimator) percentile(now time.Time) (time.Duration, int64) {
	cur, prev := e.live(now)

	var n int64
	if cur != nil {
		n += cur.total.Load()
	}
	if prev != nil {
		n += prev.total.Load()
	}
	if n < latencyWarmupSample {
		return 0, n
	}

	at := e.cachedAt.Load()
	if c := e.cached.Load(); c > 0 && now.UnixNano()-at < int64(latencyRefresh) && now.UnixNano() >= at {
		return time.Duration(c), n
	}

	d, _ := quantileOf(e.quantile, cur, prev)
	e.cached.Store(int64(d))
	e.cachedAt.Store(now.UnixNano())
	return d, n
}

// medianShift returns the median of the current and of the previous window.
// ok is false if either window has too few samples to compare.
func (e *latencyEstimator) medianShift(now time.Time) (cur, prev time.Duration, ok bool) {
	c, p := e.live(now)
	if c == nil || p == nil {
		return 0, 0, false
	}
	cd, cn := quantileOf(0.5, c)
	pd, pn := quantileOf(0.5, p)
	if cn < latencyMinSamples || pn < latencyMinSamples {
		return 0, 0, false
	}
	return cd, pd, true
}
