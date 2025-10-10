package queuelistener

import (
	"net"
	"testing"
	"time"
)

func TestNaiveStack(t *testing.T) {
	var (
		c net.Conn
		a [5]*external
	)
	for i := range len(a) {
		a[i] = &external{Conn: c}
	}
	for i := range len(a) {
		if a[i] == nil {
			t.Fatalf("a[%d] should not be nil", i)
		}
	}

	t.Run("pop before push should return nil until we get result", func(t *testing.T) {
		s := NewStack()

		ch := make(chan struct{})
		dataCH := make(chan *external)
		go func() {
			<-ch
			s.Push(a[0])
		}()
		go func() {
			v := s.Pop()
			for v == nil {
				time.Sleep(time.Millisecond)
				v = s.Pop()
			}
			dataCH <- v
		}()
		close(ch)
		if v := <-dataCH; v != a[0] {
			t.Fatalf("Failed to get item from stack: %v", v)
		}
	})

	t.Run("push pop push pop push pop ... should work", func(t *testing.T) {
		s := NewStack()
		// push pop; push pop; ..
		for i := range len(a) {
			s.Push(a[i])
			if v := s.Pop(); v != a[i] {
				t.Fatalf("Failed to get %d from stack: %v", i, v)
			}
		}
	})

	t.Run("push push push .. pop pop pop... should work", func(t *testing.T) {
		s := NewStack()
		// push push ..; pop; pop;..
		for i := range len(a) {
			s.Push(a[i])
		}
		for i := range len(a) {
			if v := s.Pop(); v != a[len(a)-i-1] {
				t.Fatalf("Failed to get a[%d] from stack: %v", len(a)-i-1, v)
			}
		}
	})

	t.Run("test push max should return without change", func(t *testing.T) {
		s := NewStack()
		s.top = len(s.items) - 2

		for i := range len(a) {
			s.Push(a[i]) // only first will be pushed on our stack rest will be ignored
		}

		if v := s.Pop(); v != a[0] {
			t.Fatalf("Failed to get a[0] from stack: %v", v)
		}
	})
}

func BenchmarkNaiveStack(b *testing.B) {
	var (
		c net.Conn
		a [5]*external
	)
	for i := range len(a) {
		a[i] = &external{Conn: c}
	}

	mystack := NewStack()
	for n := 0; n < b.N; n++ {
		for i := range len(a) {
			mystack.Push(a[i])
		}

		for i := range len(a) {
			k := mystack.Pop()
			if k != a[len(a)-1-i] {
				b.Fatalf("%p != %p", k, a[len(a)-1-i])
			}
		}
	}
}

func BenchmarkNaiveStackParallel(b *testing.B) {
	var (
		c net.Conn
		a [5]*external
	)
	for i := range 5 {
		a[i] = &external{Conn: c}
	}

	mystack := NewStack()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			for i := range len(a) {
				mystack.Push(a[i])
			}

			for range len(a) {
				k := mystack.Pop()
				if k == nil {
					b.Fatalf("%v", k)
				}
			}
		}
	})
}
