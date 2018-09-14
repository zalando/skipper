package ratelimit

import (
	"testing"
	"time"
)

type fakeSwarmer struct {
	k     string
	store map[string]map[string]interface{}
}

func newFakeSwarmer() *fakeSwarmer {
	return &fakeSwarmer{
		k:     "store",
		store: make(map[string]map[string]interface{}),
	}
}

func (f *fakeSwarmer) ShareValue(k string, v interface{}) error {
	f.store["store"][k] = v
	return nil
}
func (f *fakeSwarmer) Values(k string) map[string]interface{} {
	return f.store["store"][k]
}

func TestBackendClusterRatelimiter(t *testing.T) {
	s := Settings{
		Type:       ClusterRatelimit,
		MaxHits:    3,
		TimeWindow: 3 * time.Second,
	}
	sw := newFakeSwarmer()
	crl1 := NewClusterRateLimiter(s, sw)
	//crl2 := NewClusterRateLimiter(s, sw)
	client1 := "foo"
	//client2 := "bar"
	// waitClean := func() {
	// 	time.Sleep(s.TimeWindow)
	// }
	if !crl1.Allow(client1) {
		t.Errorf("%s not allowed but should", client1)
	}
}
