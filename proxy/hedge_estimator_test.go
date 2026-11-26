package proxy

import (
	"math"
	"math/rand/v2"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestLatencyBucketRoundTrip(t *testing.T) {
	prev := -1
	for micros := uint64(0); micros < 1<<22; micros += 1 + micros/64 {
		idx := latencyBucket(micros)
		if idx < prev {
			t.Fatalf("bucket index went down at %dus: %d after %d", micros, idx, prev)
		}
		prev = idx

		upper := latencyBucketUpper(idx)
		if upper <= time.Duration(micros)*time.Microsecond {
			t.Fatalf("upper bound %v of bucket %d is not above %dus", upper, idx, micros)
		}
		if relErr := float64(upper-time.Duration(micros)*time.Microsecond) / float64(time.Duration(micros)*time.Microsecond+1); micros >= 16 && relErr > 0.126 {
			t.Fatalf("relative error %.3f at %dus", relErr, micros)
		}
	}

	for _, micros := range []uint64{0, 1, 15, 16, 17, 1000, 1 << 39, 1<<40 - 1, 1 << 41, math.MaxUint64} {
		if idx := latencyBucket(micros); idx < 0 || idx >= latencyBuckets {
			t.Fatalf("bucket %d out of range for %dus", idx, micros)
		}
	}
}

func TestLatencyEstimatorAccuracy(t *testing.T) {
	for _, tc := range []struct {
		name string
		gen  func(*rand.Rand) time.Duration
	}{
		{"uniform 1-100ms", func(r *rand.Rand) time.Duration { return time.Duration(1+r.IntN(99_000)) * time.Microsecond }},
		{"lognormal", func(r *rand.Rand) time.Duration {
			return time.Duration(math.Exp(r.NormFloat64()*0.7+math.Log(20_000))) * time.Microsecond
		}},
		{"bimodal", func(r *rand.Rand) time.Duration {
			if r.IntN(10) == 0 {
				return 300 * time.Millisecond
			}
			return 5 * time.Millisecond
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := rand.New(rand.NewPCG(7, 8))
			e := newLatencyEstimator(0.9)
			now := time.Unix(1_700_000_010, 0)

			var all []time.Duration
			for range 20_000 {
				d := tc.gen(r)
				all = append(all, d)
				e.add(now, d)
			}
			sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
			want := all[int(0.9*float64(len(all)))]

			got, n := e.percentile(now)
			if n != 20_000 {
				t.Fatalf("n=%d", n)
			}
			if got < want || float64(got) > float64(want)*1.14 {
				t.Fatalf("p90=%v, want between %v and %v", got, want, time.Duration(float64(want)*1.14))
			}
		})
	}
}

func TestLatencyEstimatorWarmupAndCache(t *testing.T) {
	e := newLatencyEstimator(0.9)
	now := time.Unix(1_700_000_010, 0)

	for range latencyWarmupSample - 1 {
		e.add(now, 10*time.Millisecond)
	}
	if d, n := e.percentile(now); d != 0 || n != latencyWarmupSample-1 {
		t.Fatalf("warm-up: d=%v n=%d", d, n)
	}

	e.add(now, 10*time.Millisecond)
	d1, n := e.percentile(now)
	if d1 == 0 || n != latencyWarmupSample {
		t.Fatalf("after warm-up: d=%v n=%d", d1, n)
	}

	for range 1000 {
		e.add(now, 900*time.Millisecond)
	}
	if d2, _ := e.percentile(now.Add(100 * time.Millisecond)); d2 != d1 {
		t.Fatalf("the cached value was recomputed within a second: %v then %v", d1, d2)
	}
	if d3, _ := e.percentile(now.Add(2 * time.Second)); d3 <= d1 {
		t.Fatalf("the value was not refreshed after a second: %v then %v", d1, d3)
	}
}

func TestLatencyEstimatorWindowsRotate(t *testing.T) {
	e := newLatencyEstimator(0.9)
	t0 := time.Unix(1_700_000_010, 0)

	for range 500 {
		e.add(t0, 5*time.Millisecond)
	}
	d0, _ := e.percentile(t0)

	if d, n := e.percentile(t0.Add(latencyWindow)); n != 500 || d == 0 {
		t.Fatalf("the previous window should still count: d=%v n=%d", d, n)
	}
	if _, n := e.percentile(t0.Add(3 * latencyWindow)); n != 0 {
		t.Fatalf("old windows must expire: n=%d", n)
	}

	late := t0.Add(3 * latencyWindow)
	for range 500 {
		e.add(late, 400*time.Millisecond)
	}
	if d, n := e.percentile(late); n != 500 || d < 300*time.Millisecond || d <= d0 {
		t.Fatalf("a recycled window must hold only new samples: d=%v n=%d", d, n)
	}
}

func TestLatencyEstimatorMedianShift(t *testing.T) {
	e := newLatencyEstimator(0.9)
	t0 := time.Unix(1_700_000_010, 0)

	if _, _, ok := e.medianShift(t0); ok {
		t.Fatal("no data must not give a shift")
	}

	for range 200 {
		e.add(t0, 10*time.Millisecond)
	}
	t1 := t0.Add(latencyWindow)
	for range 5 {
		e.add(t1, 50*time.Millisecond)
	}
	if _, _, ok := e.medianShift(t1); ok {
		t.Fatal("a window with too few samples must not give a shift")
	}

	for range 200 {
		e.add(t1, 50*time.Millisecond)
	}
	cur, prev, ok := e.medianShift(t1)
	if !ok || cur < 4*prev || prev < 10*time.Millisecond || prev > 12*time.Millisecond {
		t.Fatalf("cur=%v prev=%v ok=%v", cur, prev, ok)
	}
}

func TestLatencyEstimatorReset(t *testing.T) {
	e := newLatencyEstimator(0.9)
	now := time.Unix(1_700_000_010, 0)
	for range 500 {
		e.add(now, 10*time.Millisecond)
	}
	e.percentile(now)
	e.reset()
	if d, n := e.percentile(now); d != 0 || n != 0 {
		t.Fatalf("after reset: d=%v n=%d", d, n)
	}
}

func TestLatencyEstimatorConcurrent(t *testing.T) {
	e := newLatencyEstimator(0.9)
	var wg sync.WaitGroup
	base := time.Unix(1_700_000_010, 0)
	for i := range 8 {
		wg.Go(func() {
			r := rand.New(rand.NewPCG(uint64(i), 1))
			for j := range 5000 {
				now := base.Add(time.Duration(j) * 20 * time.Millisecond)
				e.add(now, time.Duration(r.IntN(50_000))*time.Microsecond)
				e.percentile(now)
				e.medianShift(now)
			}
		})
	}
	wg.Wait()
}

func BenchmarkLatencyEstimator(b *testing.B) {
	now := time.Unix(1_700_000_010, 0)

	b.Run("add", func(b *testing.B) {
		e := newLatencyEstimator(0.9)
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			r := rand.New(rand.NewPCG(1, 2))
			for pb.Next() {
				e.add(now, time.Duration(r.IntN(100_000))*time.Microsecond)
			}
		})
	})

	b.Run("percentile cached", func(b *testing.B) {
		e := newLatencyEstimator(0.9)
		for i := range 5000 {
			e.add(now, time.Duration(i)*time.Microsecond)
		}
		e.percentile(now)
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				e.percentile(now)
			}
		})
	})

	b.Run("percentile recomputed", func(b *testing.B) {
		e := newLatencyEstimator(0.9)
		for i := range 5000 {
			e.add(now, time.Duration(i)*time.Microsecond)
		}
		b.ReportAllocs()
		t := now
		for b.Loop() {
			t = t.Add(2 * time.Second)
			if t.Sub(now) > latencyWindow/2 {
				t = now
			}
			e.percentile(t)
		}
	})
}
