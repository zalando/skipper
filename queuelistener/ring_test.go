package queuelistener

import (
	"net"
	"testing"
	"time"
)

type testConn int

func (c testConn) Read([]byte) (n int, err error)   { return 0, nil }
func (c testConn) Write([]byte) (n int, err error)  { return 0, nil }
func (c testConn) LocalAddr() net.Addr              { return nil }
func (c testConn) RemoteAddr() net.Addr             { return nil }
func (c testConn) SetDeadline(time.Time) error      { return nil }
func (c testConn) SetReadDeadline(time.Time) error  { return nil }
func (c testConn) SetWriteDeadline(time.Time) error { return nil }
func (c testConn) Close() error                     { return nil }

func TestRing(t *testing.T) {
	t.Run("peek", func(t *testing.T) {
		t.Run("straight", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			r.enqueue(testConn(2))
			if r.peek().(testConn) != 2 || r.size != 2 {
				t.Error("failed to peek")
			}
		})

		t.Run("straight, full", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			r.enqueue(testConn(2))
			r.enqueue(testConn(3))
			if r.peek().(testConn) != 3 || r.size != 3 {
				t.Error("failed to peek")
			}
		})

		t.Run("turned around", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			r.enqueue(testConn(2))
			r.enqueue(testConn(3))
			r.enqueue(testConn(4))
			r.enqueue(testConn(5))
			r.dequeue()
			if r.peek().(testConn) != 4 || r.size != 2 {
				t.Error("failed to peek", r.peek())
			}
		})

		t.Run("turned around, full", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			r.enqueue(testConn(2))
			r.enqueue(testConn(3))
			r.enqueue(testConn(4))
			r.enqueue(testConn(5))
			if r.peek().(testConn) != 5 || r.size != 3 {
				t.Error("failed to peek")
			}
		})
	})

	t.Run("enqueue", func(t *testing.T) {
		t.Run("straight", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			if r.enqueue(testConn(2)) != nil || r.peek().(testConn) != 2 || r.size != 2 {
				t.Error("failed to enqueue")
			}
		})

		t.Run("straight, full", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			r.enqueue(testConn(2))
			r.enqueue(testConn(3))
			if r.enqueue(testConn(4)).(testConn) != 1 || r.peek().(testConn) != 4 || r.size != 3 {
				t.Error("failed to enqueue")
			}
		})

		t.Run("turn around", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			r.enqueue(testConn(2))
			r.enqueue(testConn(3))
			r.enqueue(testConn(4))
			r.enqueue(testConn(5))
			r.dequeue()
			r.dequeue()
			if r.enqueue(testConn(6)) != nil || r.peek().(testConn) != 6 || r.size != 2 {
				t.Error("failed to enqueue")
			}
		})

		t.Run("turn around, full", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			r.enqueue(testConn(2))
			r.enqueue(testConn(3))
			r.enqueue(testConn(4))
			r.enqueue(testConn(5))
			if r.enqueue(testConn(6)).(testConn) != 3 || r.peek().(testConn) != 6 || r.size != 3 {
				t.Error("failed to enqueue")
			}
		})
	})

	t.Run("dequeue", func(t *testing.T) {
		t.Run("straight", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			r.enqueue(testConn(2))
			if r.dequeue().(testConn) != 2 || r.size != 1 {
				t.Error("failed to dequeue")
			}
		})

		t.Run("straight, full", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			r.enqueue(testConn(2))
			r.enqueue(testConn(3))
			if r.dequeue().(testConn) != 3 || r.size != 2 {
				t.Error("failed to dequeue")
			}
		})

		t.Run("turned around", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			r.enqueue(testConn(2))
			r.enqueue(testConn(3))
			r.enqueue(testConn(4))
			r.enqueue(testConn(5))
			r.dequeue()
			if r.dequeue().(testConn) != 4 || r.size != 1 {
				t.Error("failed to dequeue")
			}
		})

		t.Run("turned around, full", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			r.enqueue(testConn(2))
			r.enqueue(testConn(3))
			r.enqueue(testConn(4))
			r.enqueue(testConn(5))
			if r.dequeue().(testConn) != 5 || r.size != 2 {
				t.Error("failed to dequeue")
			}
		})
	})

	t.Run("range over", func(t *testing.T) {
		testRangeOver := func(t *testing.T, r *ring, expect []testConn) {
			r.rangeOver(func(c net.Conn) {
				if c.(testConn) != expect[0] {
					t.Error("failed to range over")
				}

				expect = expect[1:]
			})

			if len(expect) != 0 {
				t.Error("failed to range over")
			}
		}

		t.Run("straight", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			r.enqueue(testConn(2))
			testRangeOver(t, r, []testConn{1, 2})
		})

		t.Run("straight, full", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			r.enqueue(testConn(2))
			r.enqueue(testConn(3))
			testRangeOver(t, r, []testConn{1, 2, 3})
		})

		t.Run("turned over", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			r.enqueue(testConn(2))
			r.enqueue(testConn(3))
			r.enqueue(testConn(4))
			r.enqueue(testConn(5))
			r.dequeue()
			r.dequeue()
			testRangeOver(t, r, []testConn{3})
		})

		t.Run("turned over, full", func(t *testing.T) {
			r := newRing(3)
			r.enqueue(testConn(1))
			r.enqueue(testConn(2))
			r.enqueue(testConn(3))
			r.enqueue(testConn(4))
			r.enqueue(testConn(5))
			testRangeOver(t, r, []testConn{3, 4, 5})
		})
	})
}
