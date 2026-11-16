//go:build ignore

// benchmark-h3-loadgen is a standalone HTTP/3 (QUIC) load generator.
// Use it to benchmark an HTTP/3-capable server endpoint directly.
// The h2c/h2/h3 backend benchmarks in benchmark-h2c-h2-h3.sh use wrk/hey
// on the HTTP/1.1 proxy frontend; this tool is for when the proxy itself
// exposes an HTTP/3 listener.
//
// Usage:
//
//	go run benchmark-h3-loadgen.go -url https://127.0.0.1:9090/ -duration 12s -connections 128 -insecure
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go/http3"
)

func main() {
	target := flag.String("url", "https://127.0.0.1:9090/", "target URL")
	duration := flag.Duration("duration", 12*time.Second, "benchmark duration")
	connections := flag.Int("connections", 128, "number of concurrent workers")
	insecure := flag.Bool("insecure", false, "skip TLS certificate verification")
	warmup := flag.Duration("warmup", 3*time.Second, "warmup duration (requests discarded)")
	flag.Parse()

	/* #nosec */
	transport := &http3.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: *insecure},
	}
	defer transport.Close()

	client := &http.Client{Transport: transport}

	// warmup
	if *warmup > 0 {
		fmt.Fprintf(os.Stderr, "[warmup %s]\n", *warmup)
		runLoad(client, *target, *connections, *warmup, true)
	}

	fmt.Fprintf(os.Stderr, "[benchmarking %s for %s with %d workers]\n", *target, *duration, *connections)
	latencies, total, errors := runLoad(client, *target, *connections, *duration, false)

	rps := float64(total) / duration.Seconds()
	fmt.Printf("\nRequests/sec: %.2f\n", rps)
	fmt.Printf("Total requests: %d\n", total)
	fmt.Printf("Errors: %d\n", errors)
	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		fmt.Printf("Latency (ms):\n")
		fmt.Printf("  avg:  %.2f\n", avgMs(latencies))
		fmt.Printf("  50%%:  %.2f\n", pctMs(latencies, 50))
		fmt.Printf("  90%%:  %.2f\n", pctMs(latencies, 90))
		fmt.Printf("  99%%:  %.2f\n", pctMs(latencies, 99))
		fmt.Printf("  max:  %.2f\n", float64(latencies[len(latencies)-1])/float64(time.Millisecond))
	}
}

func runLoad(client *http.Client, url string, workers int, dur time.Duration, discard bool) (latencies []time.Duration, total int64, errors int64) {
	deadline := time.Now().Add(dur)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var totalAtomic, errorsAtomic atomic.Int64

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				start := time.Now()
				resp, err := client.Get(url)
				elapsed := time.Since(start)
				if err != nil {
					errorsAtomic.Add(1)
					continue
				}
				resp.Body.Close()
				totalAtomic.Add(1)
				if !discard {
					mu.Lock()
					latencies = append(latencies, elapsed)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	return latencies, totalAtomic.Load(), errorsAtomic.Load()
}

func avgMs(d []time.Duration) float64 {
	var sum float64
	for _, v := range d {
		sum += float64(v)
	}
	return sum / float64(len(d)) / float64(time.Millisecond)
}

func pctMs(sorted []time.Duration, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(len(sorted))*p/100)) - 1
	if idx < 0 {
		idx = 0
	}
	return float64(sorted[idx]) / float64(time.Millisecond)
}
