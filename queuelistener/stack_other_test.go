package queuelistener

import (
	"net"
	"testing"

	"github.com/amirylm/lockfree/core"
	lstack "github.com/amirylm/lockfree/stack"
	"github.com/golang-collections/collections/stack"
)

func TestTreiberStack(t *testing.T) {
	var c1, c2, c3 net.Conn
	s := NewTreiberStack()
	if v := s.Pop(); v != nil {
		t.Fatalf("Failed to get nil from empty stack: %v", v)
	}

	s.Push(c1)
	s.Push(c2)
	s.Push(c3)
	if v := s.Pop(); *v != c3 {
		t.Fatalf("Failed to get c3 from stack: %v", v)
	}
	if v := s.Pop(); *v != c2 {
		t.Fatalf("Failed to get c2 from stack: %v", v)
	}
	if v := s.Pop(); *v != c1 {
		t.Fatalf("Failed to get c1 from stack: %v", v)
	}

	if v := s.Pop(); v != nil {
		t.Fatalf("Failed to get nil from empty stack: %v", v)
	}
}

// https://pkg.go.dev/github.com/golang-collections/collections/stack

func BenchmarkGoStack(b *testing.B) {
	gostack := stack.New()
	for n := 0; n < b.N; n++ {
		gostack.Push(n)
		k := gostack.Pop()
		if k != n {
			b.Fatalf("%d != %d", k, n)
		}
	}
}

// https://github.com/amirylm/lockfree/blob/main/stack/stack.go

func BenchmarkAmirylmLockFreeStack(b *testing.B) {
	// func WithCapacity(c int) options.Option[Options] {
	// func New[Value any](opts ...options.Option[core.Options]) core.Stack[Value] {
	var c *net.Conn
	llstack := lstack.New[*net.Conn](core.WithCapacity(stackSize))
	for n := 0; n < b.N; n++ {
		llstack.Push(c)
		k, _ := llstack.Pop()
		if k != c {
			b.Fatalf("%p != %p", k, c)
		}
	}
}

func BenchmarkLockFreeStack(b *testing.B) {
	var c *net.Conn
	llstack := NewLockFreeStack()
	for n := 0; n < b.N; n++ {
		llstack.Push(c)
		k, _ := llstack.Pop()
		if k != c {
			b.Fatalf("%p != %p", k, c)
		}
	}
}

func BenchmarkTreiberStack(b *testing.B) {
	var c net.Conn
	mystack := NewTreiberStack()
	for n := 0; n < b.N; n++ {
		mystack.Push(c)
		k := mystack.Pop()
		if *k != c {
			b.Fatalf("%p != %p", k, &c)
		}
	}
}

func BenchmarkAmirylmLockFreeStackParallel(b *testing.B) {
	// func WithCapacity(c int) options.Option[Options] {
	// func New[Value any](opts ...options.Option[core.Options]) core.Stack[Value] {
	var c *net.Conn
	llstack := lstack.New[*net.Conn](core.WithCapacity(stackSize))
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			llstack.Push(c)
			k, _ := llstack.Pop()
			if k != c {
				b.Fatalf("%p != %p", k, c)
			}
		}
	})
}

func BenchmarkLockFreeStackParallel(b *testing.B) {
	var c *net.Conn
	llstack := NewLockFreeStack()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			llstack.Push(c)
			k, _ := llstack.Pop()
			if k != c {
				b.Fatalf("%p != %p", k, c)
			}
		}
	})
}

func BenchmarkTreiberStackParallel(b *testing.B) {
	var c net.Conn
	mystack := NewTreiberStack()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mystack.Push(c)
			k := mystack.Pop()
			if *k != c {
				b.Fatalf("%v != %v", k, c)
			}

		}
	})
}
