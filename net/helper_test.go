package net

import (
	"sync"
	"testing"

	"github.com/zalando/skipper/net/valkeytest"
)

type addressUpdater struct {
	mu    sync.Mutex
	addrs []string
	n     int
}

// update returns non empty subsequences of addrs,
// e.g. for [foo bar baz] it returns:
// 1: [foo]
// 2: [foo bar]
// 3: [foo bar baz]
// 4: [foo]
// 5: [foo bar]
// 6: [foo bar baz]
// ...
func (u *addressUpdater) update() ([]string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	result := u.addrs[0 : 1+u.n%len(u.addrs)]
	u.n++
	return result, nil
}

func (u *addressUpdater) calls() int {
	u.mu.Lock()
	defer u.mu.Unlock()

	return u.n
}

func TestAddressUpdater(t *testing.T) {
	valkeyAddr, done := valkeytest.NewTestValkey(t)
	defer done()
	valkeyAddr2, done2 := valkeytest.NewTestValkey(t)
	defer done2()

	updater := &addressUpdater{addrs: []string{valkeyAddr, valkeyAddr2}}

	if n := updater.calls(); n != 0 {
		t.Fatalf("Failed to get result from calls() want 0, got: %d", n)
	}

	addr, err := updater.update()
	if err != nil {
		t.Fatalf("Failed to update: %v", err)
	}
	if n := len(addr); n != 1 {
		t.Fatalf("Failed to get addr len of 1: %d", n)
	}
	if n := updater.calls(); n != 1 {
		t.Fatalf("Failed to get result from calls() want 1, got: %d", n)
	}

	addr, err = updater.update()
	if err != nil {
		t.Fatalf("Failed to update: %v", err)
	}
	if n := len(addr); n != 2 {
		t.Fatalf("Failed to get addr len of 2: %d", n)
	}
	if n := updater.calls(); n != 2 {
		t.Fatalf("Failed to get result from calls() want 2, got: %d", n)
	}

	addr, err = updater.update()
	if err != nil {
		t.Fatalf("Failed to update: %v", err)
	}
	if n := len(addr); n != 1 {
		t.Fatalf("Failed to get addr len of 1: %d", n)
	}
	if n := updater.calls(); n != 3 {
		t.Fatalf("Failed to get result from calls() want 3, got: %d", n)
	}
}
