// Shows Redis ratelimit groups statistics by checking given number of keys using RANDOMKEY.
//
// Example output
// ===
// ```
// $ bin/rsample -swarm-redis-urls=localhost:6379 -samples=1000
// shard                               keys
// localhost:6379                      2468
// total                               2468
//
// group                            samples    %total       min       max     50.0%     75.0%     95.0%     99.0%     99.9%
// app-one-two-three                    381      38.1         1         3         2         3         3         3         3
// app-one-two                          347      34.7         1         3         2         3         3         3         3
// app-three                            149      14.9         1         1         1         1         1         1         1
// app-two                              123      12.3         1         1         1         1         1         1         1
// ```
//
// When sampling multiple Redis shards it is important to configure shard list in exactly the same order
// it is configured in Skipper so that keys are hashed the same way, otherwise results would be wrong.
//
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/go-redis/redis/v8"
)

type keyspace struct {
	shard   string
	keys    int
	expires int
	avgTtl  int
}

type (
	histogram struct {
		name string
		v    []int64 // sorted
	}
	histogramSlice []*histogram
)

func (h histogram) Count() int                 { return len(h.v) }
func (h histogram) Min() int64                 { return h.v[0] }
func (h histogram) Max() int64                 { return h.v[len(h.v)-1] }
func (h histogram) Percentile(p float64) int64 { return h.v[int(math.Floor(p*float64(len(h.v))))] }

func (hs histogramSlice) Len() int           { return len(hs) }
func (hs histogramSlice) Swap(i, j int)      { hs[i], hs[j] = hs[j], hs[i] }
func (hs histogramSlice) Less(i, j int) bool { return hs[i].Count() > hs[j].Count() }

func main() {
	urlList := flag.String("swarm-redis-urls", "", "Redis URLs as comma separated list")
	samples := flag.Int("samples", 0, "Number of random keys to check")
	flag.Parse()

	urls := strings.Split(*urlList, ",")
	if urls[0] == "" || *samples < 0 {
		usage()
	}

	ctx := context.Background()
	ring := newRing(urls)
	defer ring.Close()

	keyspaces, err := keyspaces(ctx, ring)
	if err != nil {
		log.Fatalln(err)
	}
	histograms := measure(ctx, ring, *samples)

	report(keyspaces, histograms)
}

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
	flag.PrintDefaults()
	os.Exit(1)
}

func newRing(urls []string) *redis.Ring {
	opts := &redis.RingOptions{
		Addrs: map[string]string{},
	}
	for idx, addr := range urls {
		opts.Addrs[fmt.Sprintf("redis%d", idx)] = addr
	}
	ring := redis.NewRing(opts)
	return ring
}

func keyspaces(ctx context.Context, ring *redis.Ring) (result []keyspace, err error) {
	err = ring.ForEachShard(ctx, func(ctx context.Context, client *redis.Client) error {
		info, err := client.Info(ctx, "keyspace").Result()
		if err == nil {
			// # Keyspace
			// db0:keys=1,expires=1,avg_ttl=999336
			var ks keyspace
			_, err := fmt.Sscanf(info, "# Keyspace\ndb0:keys=%d,expires=%d,avg_ttl=%d", &ks.keys, &ks.expires, &ks.avgTtl)
			if err != nil {
				return err
			}
			ks.shard = client.Options().Addr
			result = append(result, ks)
		}
		return err
	})
	return
}

func measure(ctx context.Context, ring *redis.Ring, samples int) (hs histogramSlice) {
	hm := make(map[string]*histogram)
	for i := 0; i < samples; i++ {
		key := ring.RandomKey(ctx).Val()

		group, ok := group(key)
		if !ok {
			// ignore unrecognized keys
			continue
		}

		count, err := ring.ZCard(ctx, key).Result()
		if err != nil {
			continue
		}

		h, ok := hm[group]
		if !ok {
			h = &histogram{group, nil}
			hm[group] = h
		}
		h.v = append(h.v, count)
	}
	for _, h := range hm {
		sort.Slice(h.v, func(i, j int) bool { return h.v[i] < h.v[j] })
		hs = append(hs, h)
	}
	sort.Sort(hs)
	return
}

func group(key string) (string, bool) {
	parts := strings.Split(key, ".")
	if len(parts) < 3 || parts[0] != "ratelimit" {
		return "", false
	}
	return parts[1], true
}

func report(keyspaces []keyspace, histograms histogramSlice) {
	keysTotal := 0

	fmt.Printf("%-30s %9s\n", "shard", "keys")
	for _, ks := range keyspaces {
		fmt.Printf("%-30s %9d\n", ks.shard, ks.keys)
		keysTotal += ks.keys
	}
	fmt.Printf("%-30s %9d\n", "total", keysTotal)

	countTotal := 0
	for _, h := range histograms {
		countTotal += h.Count()
	}
	if countTotal == 0 {
		return
	}

	var percentiles = []float64{0.5, 0.75, 0.95, 0.99, 0.999}

	fmt.Printf("\n%-30s %9s %9s %9s %9s", "group", "samples", "%total", "min", "max")
	for _, p := range percentiles {
		fmt.Printf(" %8.1f%%", p*100)
	}
	fmt.Println()

	for _, h := range histograms {
		tp := float64(h.Count()*100) / float64(countTotal)

		fmt.Printf("%-30s %9d %9.1f %9d %9d", h.name, h.Count(), tp, h.Min(), h.Max())
		for _, p := range percentiles {
			fmt.Printf(" %9d", h.Percentile(p))
		}
		fmt.Println()
	}
}
