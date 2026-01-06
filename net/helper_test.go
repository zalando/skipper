package net

import (
	"sync"
)

type addressUpdater struct {
	addrs []string
	mu    sync.Mutex
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
