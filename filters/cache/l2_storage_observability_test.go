package cache

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/zalando/skipper/metrics"
	"github.com/zalando/skipper/metrics/metricstest"
)

var errL2TestMiss = errors.New("missing")
var errL2TestFailure = errors.New("client failure")

type observableL2Client struct {
	gets, sets, deletes int
	data                string
	getErr, setErr      error
	deleteErr           error
	deleted             int64
}

func (c *observableL2Client) Get(context.Context, string) (string, error) {
	c.gets++
	return c.data, c.getErr
}

func (c *observableL2Client) SetWithExpire(context.Context, string, string, time.Duration) error {
	c.sets++
	return c.setErr
}

func (c *observableL2Client) Del(context.Context, string) (int64, error) {
	c.deletes++
	return c.deleted, c.deleteErr
}

type l2Observation struct {
	operation, purpose, result string
}

type observableL2Metrics struct {
	metricstest.MockMetrics
	operations, processing []l2Observation
}

func (m *observableL2Metrics) IncCacheL2Operation(operation, purpose, outcome string) {
	m.operations = append(m.operations, l2Observation{operation, purpose, outcome})
}

func (m *observableL2Metrics) IncCacheL2ProcessingError(operation, purpose, stage string) {
	m.processing = append(m.processing, l2Observation{operation, purpose, stage})
}

func TestL2StorageObservability(t *testing.T) {
	now := time.Now()
	entry := &Entry{StatusCode: 200, CreatedAt: now, TTL: time.Minute}
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	stale := &Entry{StatusCode: 200, CreatedAt: now.Add(-2 * time.Minute), TTL: time.Minute, StaleWhileRevalidate: 3 * time.Minute}
	invalid := &Entry{CreatedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}

	purposes := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"missing", context.Background(), "unknown"},
		{"unknown", withL2Purpose(context.Background(), l2PurposeUnknown), "unknown"},
		{"response", withL2Purpose(context.Background(), l2PurposeResponse), "response"},
		{"metadata", withL2Purpose(context.Background(), l2PurposeMetadata), "metadata"},
		{"invalid", withL2Purpose(context.Background(), l2Purpose(255)), "unknown"},
		{"wrong type", context.WithValue(context.Background(), l2PurposeContextKey{}, "response"), "unknown"},
	}
	cases := []struct {
		name, action             string
		client                   observableL2Client
		l1, set                  *Entry
		wantOperation, wantStage string
		wantLegacy               string
		wantEntry, wantErr       bool
		wantFallback             bool
	}{
		{name: "get success", action: "get", client: observableL2Client{data: string(encoded)}, wantOperation: "success", wantLegacy: "cache.l2_hit", wantEntry: true},
		{name: "silent success", action: "silent", client: observableL2Client{data: string(encoded)}, wantOperation: "success", wantEntry: true},
		{name: "get miss", action: "get", client: observableL2Client{getErr: errL2TestMiss}, wantOperation: "miss", wantLegacy: "cache.l2_miss"},
		{name: "silent miss", action: "silent", client: observableL2Client{getErr: errL2TestMiss}, wantOperation: "miss"},
		{name: "get error", action: "get", client: observableL2Client{getErr: errL2TestFailure}, wantOperation: "error", wantLegacy: "cache.l2_get_error"},
		{name: "silent error", action: "silent", client: observableL2Client{getErr: errL2TestFailure}, wantOperation: "error"},
		{name: "get canceled", action: "get", client: observableL2Client{getErr: context.Canceled}, wantOperation: "error", wantLegacy: "cache.l2_get_error"},
		{name: "silent canceled", action: "silent", client: observableL2Client{getErr: context.Canceled}, wantOperation: "error"},
		{name: "get deadline exceeded", action: "get", client: observableL2Client{getErr: context.DeadlineExceeded}, wantOperation: "error", wantLegacy: "cache.l2_get_error"},
		{name: "silent deadline exceeded", action: "silent", client: observableL2Client{getErr: context.DeadlineExceeded}, wantOperation: "error"},
		{name: "get decode", action: "get", client: observableL2Client{data: "invalid json"}, wantOperation: "success", wantStage: "decode", wantErr: true},
		{name: "silent decode", action: "silent", client: observableL2Client{data: "invalid json"}, wantOperation: "success", wantStage: "decode", wantErr: true},
		{name: "get l1 hit", action: "get", l1: entry, wantLegacy: "cache.l1_hit", wantEntry: true},
		{name: "silent l1 hit", action: "silent", l1: entry, wantEntry: true},
		{name: "get stale l1", action: "get", l1: stale, client: observableL2Client{data: string(encoded)}, wantOperation: "success", wantLegacy: "cache.l2_hit", wantEntry: true},
		{name: "silent stale l1", action: "silent", l1: stale, client: observableL2Client{data: string(encoded)}, wantOperation: "success", wantEntry: true},
		{name: "set success", action: "set", set: entry, wantOperation: "success"},
		{name: "set fallback", action: "set", set: entry, client: observableL2Client{setErr: errL2TestFailure}, wantOperation: "error", wantLegacy: "cache.l2_set_fallback", wantFallback: true},
		{name: "set encode", action: "set", set: invalid, wantStage: "encode", wantErr: true},
		{name: "delete one", action: "delete", client: observableL2Client{deleted: 1}, l1: entry, wantOperation: "success"},
		{name: "delete zero", action: "delete", client: observableL2Client{deleted: 0}, l1: entry, wantOperation: "success"},
		{name: "delete error", action: "delete", client: observableL2Client{deleteErr: errL2TestFailure}, l1: entry, wantOperation: "error"},
	}

	for _, purpose := range purposes {
		for _, tc := range cases {
			t.Run(purpose.name+"/"+tc.name, func(t *testing.T) {
				m := &observableL2Metrics{}
				l1 := NewLRUStorage(64<<20, nil, metrics.Default)
				client := tc.client
				s := NewL2Storage(&client, l1, m, 0, func(err error) bool { return errors.Is(err, errL2TestMiss) })
				if tc.l1 != nil {
					if err := l1.Set(purpose.ctx, "key", tc.l1); err != nil {
						t.Fatal(err)
					}
				}

				var got *Entry
				var err error
				switch tc.action {
				case "get":
					got, err = s.Get(purpose.ctx, "key")
				case "silent":
					got, err = s.GetSilent(purpose.ctx, "key")
				case "set":
					err = s.Set(purpose.ctx, "key", tc.set)
				case "delete":
					err = s.Delete(purpose.ctx, "key")
				}
				if (err != nil) != tc.wantErr || (got != nil) != tc.wantEntry {
					t.Fatalf("entry=%v error=%v; want entry=%v error=%v", got, err, tc.wantEntry, tc.wantErr)
				}

				op := tc.action
				if op == "silent" {
					op = "get"
				}
				var wantOperations, wantProcessing []l2Observation
				if tc.wantOperation != "" {
					wantOperations = []l2Observation{{op, purpose.want, tc.wantOperation}}
				}
				if tc.wantStage != "" {
					wantProcessing = []l2Observation{{op, purpose.want, tc.wantStage}}
				}
				if !reflect.DeepEqual(m.operations, wantOperations) || !reflect.DeepEqual(m.processing, wantProcessing) {
					t.Errorf("operations=%v processing=%v; want %v %v", m.operations, m.processing, wantOperations, wantProcessing)
				}
				wantCalls := [3]int{}
				if tc.wantOperation != "" {
					switch op {
					case "get":
						wantCalls[0] = 1
					case "set":
						wantCalls[1] = 1
					case "delete":
						wantCalls[2] = 1
					}
				}
				if gotCalls := [3]int{client.gets, client.sets, client.deletes}; gotCalls != wantCalls {
					t.Errorf("client calls=%v, want %v", gotCalls, wantCalls)
				}
				m.WithMeasures(func(measures map[string][]time.Duration) {
					for i, name := range []string{"cache.l2_get_duration", "cache.l2_set_duration", "cache.l2_delete_duration"} {
						if got := len(measures[name]); got != wantCalls[i] {
							t.Errorf("%s count=%d, want %d", name, got, wantCalls[i])
						}
					}
				})
				for _, name := range []string{"cache.l1_hit", "cache.l2_hit", "cache.l2_miss", "cache.l2_get_error", "cache.l2_set_fallback"} {
					want := int64(0)
					if name == tc.wantLegacy {
						want = 1
					}
					if got, _ := m.Counter(name); got != want {
						t.Errorf("%s=%d, want %d", name, got, want)
					}
				}
				for _, result := range []struct{ name, code string }{
					{"l1_hit", "2xx"}, {"l2_hit", "2xx"}, {"l2_miss", "n/a"},
					{"l2_get_error", "n/a"}, {"l2_set_fallback", "2xx"},
				} {
					want := int64(0)
					if tc.wantLegacy == "cache."+result.name {
						want = 1
					}
					if got, _ := m.CacheResultCount(result.name, result.code); got != want {
						t.Errorf("labeled %s/%s=%d, want %d", result.name, result.code, got, want)
					}
				}
				if tc.wantStage != "" || (op == "delete" && tc.wantOperation == "error") {
					if got, _ := m.StorageErrorCount("l2", op); got != 1 {
						t.Errorf("legacy storage error %s=%d, want 1", op, got)
					}
				}
				if tc.wantFallback || op == "delete" {
					got, err := l1.Get(purpose.ctx, "key")
					if err != nil || (got != nil) != tc.wantFallback {
						t.Errorf("L1 after %s=%v, err=%v, want fallback=%v", op, got, err, tc.wantFallback)
					}
				}
			})
		}
	}
}

func BenchmarkL2StorageGetObservability(b *testing.B) {
	client := &observableL2Client{getErr: errL2TestMiss}
	s := NewL2Storage(client, NewLRUStorage(64<<20, nil, metrics.Default), metrics.Default, 0, func(err error) bool { return errors.Is(err, errL2TestMiss) })
	ctx := withL2Purpose(context.Background(), l2PurposeResponse)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.GetSilent(ctx, "missing")
	}
}
