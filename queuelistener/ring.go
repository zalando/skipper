package queuelistener

import "net"

type ring struct {
	connections []net.Conn
	next        int
	size        int
}

func newRing(maxSize int64) *ring {
	return &ring{connections: make([]net.Conn, maxSize)}
}

func (r *ring) peek() net.Conn {
	i := r.next - 1
	if i < 0 {
		i = len(r.connections) - 1
	}

	return r.connections[i]
}

func (r *ring) peekOldest() net.Conn {
	i := r.next - r.size
	if i < 0 {
		i += len(r.connections)
	}

	return r.connections[i]
}

func (r *ring) enqueue(c net.Conn) (oldest net.Conn) {
	if r.size == len(r.connections) {
		oldest = r.connections[r.next]
	} else {
		r.size++
	}

	r.connections[r.next] = c
	r.next++
	if r.next == len(r.connections) {
		r.next = 0
	}

	return
}

func (r *ring) dequeue() net.Conn {
	r.next--
	if r.next < 0 {
		r.next = len(r.connections) - 1
	}

	var c net.Conn
	c, r.connections[r.next] = r.connections[r.next], nil
	r.size--
	return c
}

func (r *ring) dequeueOldest() net.Conn {
	i := r.next - r.size
	if i < 0 {
		i += len(r.connections)
	}

	var c net.Conn
	c, r.connections[i] = r.connections[i], nil
	r.size--
	return c
}

func (r *ring) rangeOver(f func(net.Conn)) {
	start := r.next - r.size
	if start < 0 {
		start = len(r.connections) + start
	}

	finish := min(start+r.size, len(r.connections))

	for i := start; i < finish; i++ {
		f(r.connections[i])
	}

	finish = r.size + start - finish
	start = 0
	for i := start; i < finish; i++ {
		f(r.connections[i])
	}
}
