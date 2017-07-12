package proxy_test

import (
	"errors"
	"net"
	"net/http"
	"testing"
)

type failingBackend struct {
	c       chan *failingBackend
	up      bool
	healthy bool
	address string
	url     string
	server  *http.Server
	count   int
}

func freeAddress() string {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}

	defer l.Close()
	return l.Addr().String()
}

func newFailingBackend() *failingBackend {
	address := freeAddress()
	b := &failingBackend{
		c:       make(chan *failingBackend, 1),
		healthy: true,
		address: address,
		url:     "http://" + address,
	}

	b.startSynced()
	b.c <- b
	return b
}

func (b *failingBackend) synced(f func()) {
	b = <-b.c
	f()
	b.c <- b
}

func (b *failingBackend) succeed() {
	b.synced(func() {
		if b.healthy {
			return
		}

		if !b.up {
			b.startSynced()
		}

		b.healthy = true
	})
}

func (b *failingBackend) fail() {
	b.synced(func() {
		b.healthy = false
	})
}

func (b *failingBackend) counter() int {
	var count int
	b.synced(func() {
		count = b.count
	})

	return count
}

func (b *failingBackend) resetCounter() {
	b.synced(func() {
		b.count = 0
	})
}

func (b *failingBackend) startSynced() {
	if b.up {
		return
	}

	l, err := net.Listen("tcp", b.address)
	if err != nil {
		panic(err)
	}

	b.server = &http.Server{Handler: b}

	b.up = true
	go func(s *http.Server, l net.Listener) {
		err := s.Serve(l)
		if err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}(b.server, l)
}

func (b *failingBackend) start() {
	b.synced(b.startSynced)
}

func (b *failingBackend) closeSynced() {
	if !b.up {
		return
	}

	b.server.Close()
	b.up = false
}

func (b *failingBackend) close() {
	b.synced(b.closeSynced)
}

func (b *failingBackend) down() { b.close() }

func (b *failingBackend) reset() {
	b.synced(func() {
		b.closeSynced()
		b.count = 0
		b.startSynced()
	})
}

func (b *failingBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.synced(func() {
		b.count++
		if !b.healthy {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
}

func TestFailingBackend(t *testing.T) {
	b := newFailingBackend()
	defer b.close()

	req := func(fail, down bool) error {
		rsp, err := http.Get(b.url)
		if down {
			if err == nil {
				return errors.New("failed to fail")
			}

			return nil
		} else if err != nil {
			return err
		}

		defer rsp.Body.Close()

		if fail && rsp.StatusCode != http.StatusInternalServerError ||
			!fail && rsp.StatusCode != http.StatusOK {
			t.Error("invalid status", rsp.StatusCode)
		}

		return nil
	}

	if err := req(false, false); err != nil {
		t.Error(err)
		return
	}

	b.fail()
	if err := req(true, false); err != nil {
		t.Error(err)
		return
	}

	b.succeed()
	if err := req(false, false); err != nil {
		t.Error(err)
		return
	}

	b.fail()
	if err := req(true, false); err != nil {
		t.Error(err)
		return
	}

	b.down()
	if err := req(false, true); err != nil {
		t.Error(err)
		return
	}

	b.start()
	if err := req(true, false); err != nil {
		t.Error(err)
		return
	}

	b.succeed()
	if err := req(false, false); err != nil {
		t.Error(err)
		return
	}
}
