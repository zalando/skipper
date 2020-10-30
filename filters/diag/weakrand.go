package diag

import (
	"math/rand"
	"sync"
)

type reader struct {
	c  []byte
	i  int
	mx *sync.Mutex
}

var randomChars = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")

func weakRandom() reader {
	c := make([]byte, len(randomChars))
	copy(c, randomChars)
	rand.Shuffle(len(c), func(i, j int) {
		c[i], c[j] = c[j], c[i]
	})

	return reader{c: c, mx: &sync.Mutex{}}
}

func (r reader) Read(p []byte) (int, error) {
	r.mx.Lock()
	i := r.i
	r.mx.Unlock()

	var n int
	for len(p) > 0 {
		l := copy(p, r.c[i:])
		p = p[l:]
		n += l
		i += l
		if i >= len(r.c) {
			i = 0
		}
	}

	r.mx.Lock()
	r.i = i
	r.mx.Unlock()
	return n, nil
}
