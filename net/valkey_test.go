package net

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/net/valkeytest"
	"github.com/zalando/skipper/tracing/tracers/basic"
)

func TestValkeyContainer(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()
	if valkeyAddr == "" {
		t.Fatal("Failed to create valkey 1")
	}
	valkeyAddr2, done2 := valkeytest.NewTestValkey(t)
	defer done2()
	if valkeyAddr2 == "" {
		t.Fatal("Failed to create valkey 2")
	}
}

func TestValkey_hasAll(t *testing.T) {
	for _, tt := range []struct {
		name string
		a    []string
		h    map[string]struct{}
		want bool
	}{
		{
			name: "both empty",
			a:    nil,
			h:    nil,
			want: true,
		},
		{
			name: "a empty",
			a:    nil,
			h: map[string]struct{}{
				"foo": {},
			},
			want: false,
		},
		{
			name: "h empty",
			a:    []string{"foo"},
			h:    nil,
			want: false,
		},
		{
			name: "both set equal",
			a:    []string{"foo"},
			h: map[string]struct{}{
				"foo": {},
			},
			want: true,
		},
		{
			name: "both set notequal",
			a:    []string{"fo"},
			h: map[string]struct{}{
				"foo": {},
			},
			want: false,
		},
		{
			name: "both set multiple equal",
			a:    []string{"bar", "foo"},
			h: map[string]struct{}{
				"foo": {},
				"bar": {},
			},
			want: true,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			got := hasAll(tt.a, tt.h)
			if tt.want != got {
				t.Fatalf("Failed to get %v for hasall(%v, %v)", tt.want, tt.a, tt.h)
			}
		})
	}
}
func TestValkeyScript(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()
	cli, err := NewValkeyRingClient(&ValkeyOptions{
		Addrs:         []string{valkeyAddr},
		Metrics:       &metricstest.MockMetrics{},
		MetricsPrefix: "skipper.valkey.",
	})
	if err != nil {
		t.Fatalf("Failed to create valkey ring client: %v", err)
	}
	defer func() {
		t.Logf("closing ring client")
		cli.Close()
	}()

	src := "return {KEYS[1],KEYS[2],ARGV[1],ARGV[2]}"
	l := NewScript(src)
	msg, err := cli.RunScript(context.Background(), l, []string{"k1", "k2"}, "arg1", "arg2")
	if err != nil {
		t.Fatalf("Failed to run script %q: %v", src, err)
	}

	a, err := msg.ToArray()
	if err != nil {
		t.Fatalf("Failed to get result: %v", err)
	}
	res := make([]string, 0, 4)
	for _, msg := range a {
		s, err := msg.ToString()
		if err != nil {
			t.Fatalf("Failed ToString: %v", err)
		}
		res = append(res, s)
	}
	if got := strings.Join(res, "|"); got != "k1|k2|arg1|arg2" {
		t.Fatalf(`Failed to get expected result "k1|k2|arg1|arg2", got: %q`, got)
	}

}

func TestValkeyPing(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()
	vrc, err := NewValkeyRingClient(&ValkeyOptions{
		Addrs:         []string{valkeyAddr},
		Metrics:       &metricstest.MockMetrics{},
		MetricsPrefix: "skipper.valkey.",
	})
	if err != nil {
		t.Fatalf("Failed to create valkey ring client: %v", err)
	}
	defer func() {
		vrc.Close()
	}()

	err = vrc.Ping(context.Background(), valkeyAddr)
	if err != nil {
		t.Fatalf("Failed to ping %q: %v", valkeyAddr, err)
	}
}

func TestValkeyClientSetAddr(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()
	valkeyAddr2, done2 := valkeytest.NewTestValkey(t)
	defer done2()
	vrc, err := NewValkeyRingClient(&ValkeyOptions{
		Addrs: []string{valkeyAddr, valkeyAddr2},
	})
	if err != nil {
		t.Fatalf("Failed to create valkey ring client: %v", err)
	}
	defer func() {
		t.Logf("closing ring client")
		vrc.Close()
	}()

	time.Sleep(100 * time.Millisecond)
	if n := vrc.ring.Len(); n != 2 {
		t.Fatalf("Failed to get number of shards: %d != %d", n, 2)
	}

	// call path that check it has nothing todo
	err = vrc.ring.SetAddr([]string{valkeyAddr, valkeyAddr2})
	if err != nil {
		t.Fatalf("Failed to ring.SetAddr with the same addresses: %v", err)
	}
	if n := vrc.ring.Len(); n != 2 {
		t.Fatalf("Failed to get number of shards: %d != %d", n, 2)
	}

	vrc.SetAddrs(context.Background(), []string{valkeyAddr})
	time.Sleep(100 * time.Millisecond)
	if n := vrc.ring.Len(); n != 1 {
		t.Fatalf("Failed to get number of shards: %d != %d", n, 1)
	}

	vrc.SetAddrs(context.Background(), []string{valkeyAddr, valkeyAddr2})
	if n := vrc.ring.Len(); n != 2 {
		t.Fatalf("Failed to get number of shards: %d != %d", n, 2)
	}
}

func TestValkeyClientAddDeleteInstance(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()
	valkeyAddr2, done2 := valkeytest.NewTestValkey(t)
	defer done2()
	valkeyAddr3, done3 := valkeytest.NewTestValkey(t)
	defer done3()

	t.Log("#####################################")
	updater := &addressUpdater{addrs: []string{valkeyAddr, valkeyAddr2, valkeyAddr3}}
	cli, err := NewValkeyRingClient(&ValkeyOptions{
		Addrs:          []string{valkeyAddr, valkeyAddr2},
		AddrUpdater:    updater.update,
		UpdateInterval: 100 * time.Millisecond,
		Metrics:        &metricstest.MockMetrics{},
		MetricsPrefix:  "skipper.valkey.",
	})
	if err != nil {
		t.Fatalf("Failed to create valkey ring client: %v", err)
	}
	defer func() {
		t.Logf("closing ring client")
		cli.Close()
	}()

	// initial wait for startUpdater to run once
	time.Sleep(cli.options.UpdateInterval + 50*time.Millisecond)

	for range 5 {
		updater.update()
		time.Sleep(cli.options.UpdateInterval)

		mock := cli.metrics.(*metricstest.MockMetrics)
		mock.WithGauges(func(m map[string]float64) {
			key := cli.metricsPrefix + "shards"
			if _, ok := m[key]; ok {
				for k, v := range m {
					t.Logf("metric %s: %v", k, v)
				}
			}
		})
	}
}

func TestValkeyClient(t *testing.T) {
	tracer, err := basic.InitTracer([]string{"recorder=in-memory"})
	if err != nil {
		t.Fatalf("Failed to get a tracer: %v", err)
	}
	defer tracer.Close()

	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()
	valkeyAddr2, done2 := valkeytest.NewTestValkey(t)
	defer done2()

	updater := &addressUpdater{addrs: []string{valkeyAddr, valkeyAddr2}}

	for _, tt := range []struct {
		name    string
		options *ValkeyOptions
		wantErr bool
	}{
		{
			name: "All defaults",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			wantErr: false,
		},
		{
			name: "With tracer",
			options: &ValkeyOptions{
				Addrs:  []string{valkeyAddr},
				Tracer: tracer,
			},
			wantErr: false,
		},
		{
			name: "With metrics and AddrUpdater",
			options: &ValkeyOptions{
				Addrs:          []string{valkeyAddr},
				AddrUpdater:    updater.update,
				UpdateInterval: 100 * time.Millisecond,
				Metrics:        &metricstest.MockMetrics{},
				MetricsPrefix:  "skipper.valkey.",
			},
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli, err := NewValkeyRingClient(tt.options)
			if err != nil {
				if !tt.wantErr {
					t.Fatalf("Failed to create ring client: %v", err)
				}
			}

			defer func() {
				if !cli.closed {
					t.Error("Failed to close valkey ring client")
				}
			}()
			defer func() {
				t.Logf("closing ring client")
				cli.Close()
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if !cli.RingAvailable(ctx) {
				t.Logf("ring is not available")
				if tt.options.AddrUpdater != nil {
					t.Log("Inittially we have no connected valkey client, ring not available")
				} else {
					t.Fatalf("Failed to have a connected valkey client, ring not available")
				}
			} else {
				t.Logf("ring is available")
			}

			if tt.options.Tracer != nil {
				span := cli.StartSpan("test")
				span.Finish()
			}

			if tt.options.Metrics != nil {
				updater.update()
				updater.update()
				time.Sleep(2 * cli.options.UpdateInterval)

				if mock, ok := cli.metrics.(*metricstest.MockMetrics); ok {
					mock.WithGauges(func(m map[string]float64) {
						key := tt.options.MetricsPrefix + "shards"
						if v, ok := m[key]; !ok {
							t.Fatalf("Failed to get metric %q", key)
						} else {
							for k, v := range m {
								t.Logf("metric %s: %v", k, v)
							}
							i := int(v)
							if i != 2 {
								t.Fatalf("Failed to get 2 shards, got: %d", i)
							}
						}
					})

					for range 15 {
						a, err := updater.update()
						if err != nil {
							t.Fatalf("Failed to update list: %v", err)
						}
						t.Logf("updated: %v", a)
						time.Sleep(cli.options.UpdateInterval)

						mock.WithGauges(func(m map[string]float64) {
							key := tt.options.MetricsPrefix + "shards"
							if _, ok := m[key]; ok {
								for k, v := range m {
									t.Logf("metric %s: %v", k, v)
								}
							}
						})
					}
				}
			}

			if tt.options.AddrUpdater != nil {
				// test address updater is called
				initial := updater.calls()
				t.Logf("cli.options.UpdateInterval: %s", cli.options.UpdateInterval)

				time.Sleep(2 * cli.options.UpdateInterval)

				if updater.calls() == initial {
					t.Errorf("expected updater call")
				}

				// test close stops background update
				cli.Close()

				time.Sleep(2 * cli.options.UpdateInterval)

				afterClose := updater.calls()

				time.Sleep(2 * cli.options.UpdateInterval)

				if updater.calls() != afterClose {
					t.Errorf("expected no updater call")
				}

				if !cli.closed {
					t.Error("Failed to close")
				}
			}
		})
	}
}

func TestValkeyClientGetSet(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()

	for _, tt := range []struct {
		name    string
		options *ValkeyOptions
		key     string
		value   string
		expect  string
		wantErr bool
	}{
		{
			name: "add none",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			wantErr: true,
		},
		{
			name: "add one, get one, no expiration",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key:     "k1",
			value:   "foo",
			expect:  "foo",
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli, err := NewValkeyRingClient(tt.options)
			if err != nil {
				t.Fatalf("Failed to create valkey ring: %v", err)
			}

			defer cli.Close()
			ctx := context.Background()

			_, err = cli.Set(ctx, tt.key, tt.value)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do Set error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			val, err := cli.Get(ctx, tt.key)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do Get error = %v, wantErr %v", err, tt.wantErr)
			}
			if val != tt.expect {
				t.Errorf("Failed to get correct Get value, want '%v', got '%v'", tt.expect, val)
			}
		})
	}
}

func TestValkeyClientGetSetWithExpire(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()

	for _, tt := range []struct {
		name    string
		options *ValkeyOptions
		key     string
		value   string
		expire  time.Duration
		wait    time.Duration
		expect  string
		wantErr bool
	}{
		{
			name: "add none",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			wantErr: true,
		},
		{
			name: "add one, get one, no expiration",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key:     "k1",
			value:   "foo",
			expect:  "foo",
			wantErr: false,
		},
		{
			name: "add one, get one, with expiration",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key:     "k1",
			value:   "foo",
			expire:  time.Second,
			expect:  "foo",
			wantErr: false,
		},
		{
			name: "add one, get none, with expiration, wait to expire",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key:     "k1",
			value:   "foo",
			expire:  time.Second,
			wait:    1100 * time.Millisecond,
			wantErr: true,
		},
		{
			name: "add one, get one, no expiration, with Rendezvous hash",
			options: &ValkeyOptions{
				Addrs:         []string{valkeyAddr},
				HashAlgorithm: "rendezvous",
			},
			key:     "k1",
			value:   "foo",
			expire:  time.Second,
			expect:  "foo",
			wantErr: false,
		},
		{
			name: "add one, get one, no expiration, with Rendezvous Vnodes hash",
			options: &ValkeyOptions{
				Addrs:         []string{valkeyAddr},
				HashAlgorithm: "rendezvousVnodes",
			},
			key:     "k1",
			value:   "foo",
			expire:  time.Second,
			expect:  "foo",
			wantErr: false,
		},
		{
			name: "add one, get one, no expiration, with Jump hash",
			options: &ValkeyOptions{
				Addrs:         []string{valkeyAddr},
				HashAlgorithm: "jump",
			},
			key:     "k1",
			value:   "foo",
			expire:  time.Second,
			expect:  "foo",
			wantErr: false,
		},
		{
			name: "add one, get one, no expiration, with Multiprobe hash",
			options: &ValkeyOptions{
				Addrs:         []string{valkeyAddr},
				HashAlgorithm: "mpchash",
			},
			key:     "k1",
			value:   "foo",
			expire:  time.Second,
			expect:  "foo",
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli, err := NewValkeyRingClient(tt.options)
			if err != nil {
				t.Fatalf("Failed to create valkey ring: %v", err)
			}

			defer cli.Close()
			ctx := context.Background()

			if tt.expire == 0 {
				t.Logf("expire not set so set it to arbitrary large value")
				tt.expire = time.Hour
			}
			err = cli.SetWithExpire(ctx, tt.key, tt.value, tt.expire)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do Set error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			time.Sleep(tt.wait)

			val, err := cli.Get(ctx, tt.key)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do Get error = %v, wantErr %v", err, tt.wantErr)
			}
			if val != tt.expect {
				t.Errorf("Failed to get correct Get value, want '%v', got '%v'", tt.expect, val)
			}
		})
	}
}

func TestValkeyClientZAddZCard(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()

	type valScore struct {
		val   string
		score float64
	}
	for _, tt := range []struct {
		name    string
		options *ValkeyOptions
		h       map[string][]valScore
		key     string
		zcard   int64
		wantErr bool
	}{
		{
			name: "add none",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			wantErr: true,
		},
		{
			name: "add one",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   "10",
						score: 5.0,
					},
				},
			},
			zcard:   1,
			wantErr: false,
		},
		{
			name: "add one more values",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k2",
			h: map[string][]valScore{
				"k2": {
					{
						val:   "1",
						score: 1.0,
					}, {
						val:   "2",
						score: 2.0,
					}, {
						val:   "3",
						score: 3.0,
					},
				},
			},
			zcard:   3,
			wantErr: false,
		},
		{
			name: "add 2 keys and values",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   "1",
						score: 1.0,
					}, {
						val:   "2",
						score: 2.0,
					}, {
						val:   "3",
						score: 3.0,
					},
				},
				"k2": {
					{
						val:   "1",
						score: 1.0,
					},
				},
			},
			zcard:   3,
			wantErr: false,
		},
		{
			name: "add 2 keys and values",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k2",
			h: map[string][]valScore{
				"k1": {
					{
						val:   "1",
						score: 1.0,
					}, {
						val:   "2",
						score: 2.0,
					}, {
						val:   "3",
						score: 3.0,
					},
				},
				"k2": {
					{
						val:   "1",
						score: 1.0,
					},
				},
			},
			zcard:   1,
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli, err := NewValkeyRingClient(tt.options)
			if err != nil {
				t.Fatalf("Failed to create valkey ring client: %v", err)
			}

			defer cli.Close()
			ctx := context.Background()

			for k, a := range tt.h {
				for _, v := range a {
					_, err = cli.ZAdd(ctx, k, v.val, v.score)
					if err != nil && !tt.wantErr {
						t.Errorf("Failed to do ZAdd error = %v, wantErr %v", err, tt.wantErr)
					}
					if tt.wantErr {
						return
					}
				}
			}

			val, err := cli.ZCard(ctx, tt.key)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZCard error = %v, wantErr %v", err, tt.wantErr)
			}
			if val != tt.zcard {
				t.Errorf("Failed to get correct ZCard value, want %d, got %d", tt.zcard, val)
			}

			// cleanup
			for k, a := range tt.h {
				for _, v := range a {
					_, err = cli.ZRem(ctx, k, v.val)
					if err != nil {
						t.Errorf("Failed to remove key %s: %v", tt.key, err)
					}
				}
			}

		})
	}
}

func TestValkeyClientExpire(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()

	type valScore struct {
		val   string
		score float64
	}
	for _, tt := range []struct {
		name             string
		options          *ValkeyOptions
		h                map[string][]valScore
		key              string
		wait             time.Duration
		expire           time.Duration // >=1s, because Valkey
		zcard            int64
		zcardAfterExpire int64
		wantErr          bool
	}{
		{
			name: "add none",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			zcard:            0,
			zcardAfterExpire: 0,
			wantErr:          false,
		},
		{
			name: "add one which does not expire",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   "10",
						score: 5.0,
					},
				},
			},
			expire:           time.Second,
			zcard:            1,
			zcardAfterExpire: 1,
			wantErr:          false,
		},
		{
			name: "add one which does expire",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   "10",
						score: 5.0,
					},
				},
			},
			expire:           1 * time.Second,
			wait:             1100 * time.Millisecond,
			zcard:            1,
			zcardAfterExpire: 0,
			wantErr:          false,
		},
		{
			name: "add one more values expire all",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k2",
			h: map[string][]valScore{
				"k2": {
					{
						val:   "1",
						score: 1.0,
					}, {
						val:   "2",
						score: 2.0,
					}, {
						val:   "3",
						score: 3.0,
					},
				},
			},
			expire:           1 * time.Second,
			wait:             1100 * time.Millisecond,
			zcard:            3,
			zcardAfterExpire: 0,
			wantErr:          false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli, err := NewValkeyRingClient(tt.options)
			if err != nil {
				t.Fatalf("Failed to create valkey ring client: %v", err)
			}

			defer cli.Close()
			ctx := context.Background()

			for k, a := range tt.h {
				for _, v := range a {
					_, err := cli.ZAdd(ctx, k, v.val, v.score)
					if err != nil && !tt.wantErr {
						t.Errorf("Failed to do ZAdd error = %v, wantErr %v", err, tt.wantErr)
					}
					if tt.wantErr {
						return
					}

				}
				cli.Expire(ctx, k, tt.expire)
			}

			val, err := cli.ZCard(ctx, tt.key)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZCard error = %v, wantErr %v", err, tt.wantErr)
			}
			if val != tt.zcard {
				t.Errorf("Failed to get correct ZCard value, want %d, got %d", tt.zcard, val)
			}

			time.Sleep(tt.wait)

			val, err = cli.ZCard(ctx, tt.key)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZCard error = %v, wantErr %v", err, tt.wantErr)
			}
			if val != tt.zcardAfterExpire {
				t.Errorf("Failed to get correct ZCard value, want %d, got %d", tt.zcardAfterExpire, val)
			}

		})
	}
}

func TestValkeyClientZRemRangeByScore(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()

	type valScore struct {
		val   string
		score float64
	}
	for _, tt := range []struct {
		name          string
		options       *ValkeyOptions
		h             map[string][]valScore
		key           string
		delScoreMin   string
		delScoreMax   string
		zcard         int64
		zcardAfterRem int64
		wantErr       bool
	}{
		{
			name: "none",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			wantErr: true,
		},
		{
			name: "delete none",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   "10",
						score: 5.0,
					},
				},
			},
			zcard:         1,
			zcardAfterRem: 1,
			delScoreMin:   "6.0",
			delScoreMax:   "7.0",
			wantErr:       false,
		},
		{
			name: "delete one",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   "10",
						score: 5.0,
					},
				},
			},
			zcard:         1,
			zcardAfterRem: 0,
			delScoreMin:   "1.0",
			delScoreMax:   "7.0",
			wantErr:       false,
		},
		{
			name: "delete one have more values",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k2",
			h: map[string][]valScore{
				"k2": {
					{
						val:   "1",
						score: 1.0,
					}, {
						val:   "2",
						score: 2.0,
					}, {
						val:   "3",
						score: 3.0,
					},
				},
			},
			zcard:         3,
			zcardAfterRem: 2,
			delScoreMin:   "1.0",
			delScoreMax:   "1.5",
			wantErr:       false,
		},
		{
			name: "delete 2 have more values offset score",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k2",
			h: map[string][]valScore{
				"k2": {
					{
						val:   "1",
						score: 1.0,
					}, {
						val:   "2",
						score: 2.0,
					}, {
						val:   "3",
						score: 3.0,
					},
				},
			},
			zcard:         3,
			zcardAfterRem: 1,
			delScoreMin:   "2.0",
			delScoreMax:   "5.0",
			wantErr:       false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli, err := NewValkeyRingClient(tt.options)
			if err != nil {
				t.Fatalf("Failed to create valkey ring client: %v", err)
			}

			defer cli.Close()
			ctx := context.Background()

			for k, a := range tt.h {
				for _, v := range a {
					_, err := cli.ZAdd(ctx, k, v.val, v.score)
					if err != nil && !tt.wantErr {
						t.Errorf("Failed to do ZAdd error = %v, wantErr %v", err, tt.wantErr)
					}
					if tt.wantErr {
						return
					}
				}
			}

			val, err := cli.ZCard(ctx, tt.key)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZCard error = %v, wantErr %v", err, tt.wantErr)
			}
			if val != tt.zcard {
				t.Errorf("Failed to get correct ZCard value, want %d, got %d", tt.zcard, val)
			}

			_, err = cli.ZRemRangeByScore(ctx, tt.key, tt.delScoreMin, tt.delScoreMax)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZRemRangeByScore error = %v, wantErr %v", err, tt.wantErr)
			}

			val, err = cli.ZCard(ctx, tt.key)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZCard error = %v, wantErr %v", err, tt.wantErr)
			}
			if val != tt.zcardAfterRem {
				t.Errorf("Failed to get correct ZCard value, want %d, got %d", tt.zcard, val)
			}

			// cleanup
			for k, a := range tt.h {
				for _, v := range a {
					_, err = cli.ZRem(ctx, k, v.val)
					if err != nil {
						t.Errorf("Failed to remove key %s: %v", tt.key, err)
					}
				}
			}

		})
	}
}

func TestValkeyClientZRangeByScoreWithScoresFirst(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()

	type valScore struct {
		val   string
		score float64
	}
	for _, tt := range []struct {
		name     string
		options  *ValkeyOptions
		h        map[string][]valScore
		key      string
		min      string
		max      string
		offset   int64
		count    int64
		expected string
		wantErr  bool
	}{
		{
			name: "none",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			wantErr: true,
		},
		{
			name: "one key, have one value, get first by min max",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   "10",
						score: 5.0,
					},
				},
			},
			min:      "1.0",
			max:      "7.0",
			expected: "10",
			wantErr:  false,
		},
		{
			name: "one key, have one value, get none by min max",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   "10",
						score: 5.0,
					},
				},
			},
			min:     "6.0",
			max:     "7.0",
			wantErr: false,
		},
		{
			name: "one key, have one value, get none by offset",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k1",
			h: map[string][]valScore{
				"k1": {
					{
						val:   "10",
						score: 5.0,
					},
				},
			},
			min:     "1.0",
			max:     "7.0",
			offset:  3,
			wantErr: false,
		},
		{
			name: "one key, have more values, get last by offset",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k2",
			h: map[string][]valScore{
				"k2": {
					{
						val:   "1",
						score: 1.0,
					}, {
						val:   "2",
						score: 2.0,
					}, {
						val:   "3",
						score: 3.0,
					},
				},
			},
			min:      "1.0",
			max:      "5.0",
			offset:   2,
			count:    10,
			expected: "3",
			wantErr:  false,
		},
		{
			name: "one key, have more values, get second by offset",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k2",
			h: map[string][]valScore{
				"k2": {
					{
						val:   "1",
						score: 1.0,
					}, {
						val:   "2",
						score: 2.0,
					}, {
						val:   "3",
						score: 3.0,
					},
				},
			},
			min:      "1.0",
			max:      "5.0",
			offset:   1,
			count:    10,
			expected: "2",
			wantErr:  false,
		},
		{
			name: "one key, have more values, select all get first",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr},
			},
			key: "k2",
			h: map[string][]valScore{
				"k2": {
					{
						val:   "1",
						score: 1.0,
					}, {
						val:   "2",
						score: 2.0,
					}, {
						val:   "3",
						score: 3.0,
					},
				},
			},
			min:      "0.0",
			max:      "5.0",
			offset:   0,
			count:    10,
			expected: "1",
			wantErr:  false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cli, err := NewValkeyRingClient(tt.options)
			if err != nil {
				t.Fatalf("Failed to create valkey ring client: %v", err)
			}

			defer cli.Close()
			ctx := context.Background()

			for k, a := range tt.h {
				for _, v := range a {
					res, err := cli.ZAdd(ctx, k, v.val, v.score)
					if err != nil && !tt.wantErr {
						t.Fatalf("Failed to do ZAdd error = %v, wantErr %v", err, tt.wantErr)
					}
					t.Logf("ZADD res: %d", res)
					if tt.wantErr {
						return
					}
				}
			}

			res, err := cli.ZRangeByScoreWithScoresFirst(ctx, tt.key, tt.min, tt.max, tt.offset, tt.count)
			if err != nil && !tt.wantErr {
				t.Errorf("Failed to do ZRangeByScoreWithScoresFirst error = %v, wantErr %v", err, tt.wantErr)
			}

			// TODO(sszuecs): whatever we expect to be reutrned by the CMD above
			_ = res // no build error
			// if tt.expected == "" {
			// 	if res != nil {
			// 		t.Errorf("Expected nil got: '%v'", res)
			// 	}
			// } else {
			// 	diff := cmp.Diff(res, tt.expected)
			// 	if diff != "" {
			// 		t.Error(diff)
			// 	}
			// }

			// cleanup
			for k, a := range tt.h {
				for _, v := range a {
					_, err = cli.ZRem(ctx, k, v.val)
					if err != nil {
						t.Errorf("Failed to remove key %s: %v", tt.key, err)
					}
				}
			}

		})
	}
}

func TestValkeyClientCommandsOnSetAddrUpdate(t *testing.T) {
	valkeyAddr1, done1 := valkeytest.NewTestValkey(t)
	defer done1()
	valkeyAddr2, done2 := valkeytest.NewTestValkey(t)
	defer done2()

	for _, tt := range []struct {
		name         string
		options      *ValkeyOptions
		valkeyUpdate []string
		keys         []string
		vals         []string
	}{
		{
			name: "no valkey change",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr1, valkeyAddr2},
			},
			keys: []string{"foo1", "foo2", "foo3", "foo4", "foo5"},
			vals: []string{"bar1", "bar2", "bar3", "bar4", "bar5"},
		},
		{
			name: "with valkey add",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr1},
			},
			valkeyUpdate: []string{
				valkeyAddr1,
				valkeyAddr2,
			},
			keys: []string{"foo1", "foo2", "foo3", "foo4", "foo5"},
			vals: []string{"bar1", "bar2", "bar3", "bar4", "bar5"},
		},
		{
			name: "with valkey del",
			options: &ValkeyOptions{
				Addrs: []string{valkeyAddr1, valkeyAddr2},
			},
			valkeyUpdate: []string{
				valkeyAddr1,
			},
			keys: []string{"foo1", "foo2", "foo3", "foo4", "foo5"},
			vals: []string{"bar1", "bar2", "bar3", "bar4", "bar5"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewValkeyRingClient(tt.options)
			if err != nil {
				t.Fatalf("Failed to create valkey ring client: %v", err)
			}

			defer r.Close()
			for i := 0; i < len(tt.keys); i++ {
				err = r.SetWithExpire(context.Background(), tt.keys[i], tt.vals[i], time.Second)
				if err != nil {
					t.Fatalf("Failed to SetWithExpire: %v", err)
				}

			}
			if len(tt.valkeyUpdate) != len(tt.options.Addrs) {
				r.SetAddrs(context.Background(), tt.valkeyUpdate)
			}
			for i := 0; i < len(tt.keys); i++ {
				got, err := r.Get(context.Background(), tt.keys[i])
				if err != nil {
					// can happen after updated shards, so retry
					err = r.SetWithExpire(context.Background(), tt.keys[i], tt.vals[i], time.Second)
					if err != nil {
						t.Fatalf("Failed to SetWithExpire: %v", err)
					}
					got, err = r.Get(context.Background(), tt.keys[i])
					if err != nil {
						t.Fatalf("Failed to Get: %v", err)
					}
				}
				if got != tt.vals[i] {
					t.Errorf("Failed to get key '%s' wanted '%s', got '%s'", tt.keys[i], tt.vals[i], got)
				}
			}
		})
	}
}

func TestValkeyClientFailingAddrUpdater(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cli, err := NewValkeyRingClient(&ValkeyOptions{
			AddrUpdater: func() ([]string, error) {
				return nil, fmt.Errorf("failed to get addresses")
			},
			UpdateInterval: 1 * time.Second,
		})
		if err != nil {
			t.Fatalf("Failed to createvalkeyring client: %v", err)
		}

		defer cli.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if cli.RingAvailable(ctx) {
			t.Error("Unexpected available ring")
		}
	})
}

func BenchmarkShardForKey(b *testing.B) {
	valkeyAddr1, done1 := valkeytest.NewTestValkey(b)
	defer done1()
	valkeyAddr2, done2 := valkeytest.NewTestValkey(b)
	defer done2()

	options := &ValkeyOptions{
		Addrs: []string{valkeyAddr1, valkeyAddr2},
	}
	r, err := NewValkeyRingClient(options)
	if err != nil {
		b.Fatalf("Failed to create valkey ring client: %v", err)
	}
	defer r.Close()

	b.ResetTimer()

	for b.Loop() {
		r.ring.shardForKey("A") // 9ns
	}
}
