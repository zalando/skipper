package queuelistener

import (
	"net"
	"sync/atomic"
	"unsafe"
)

// https://englyk.com/book2/Lock-Free_Data_Structures/

// Node represents a node in the stack
type Node struct {
	value any
	next  *Node
}

// LockFreeStack represents a lock-free stack
type LockFreeStack struct {
	head unsafe.Pointer // *Node
}

// NewLockFreeStack creates a new lock-free stack
func NewLockFreeStack() *LockFreeStack {
	return &LockFreeStack{}
}

// Push adds a new value onto the stack
func (s *LockFreeStack) Push(value interface{}) {
	newNode := &Node{value: value}
	for {
		oldHead := atomic.LoadPointer(&s.head)
		newNode.next = (*Node)(oldHead)
		if atomic.CompareAndSwapPointer(&s.head, oldHead, unsafe.Pointer(newNode)) {
			break
		}
	}
}

// Pop removes and returns the value from the top of the stack
func (s *LockFreeStack) Pop() (value any, ok bool) {
	for {
		oldHead := atomic.LoadPointer(&s.head)
		if oldHead == nil {
			return nil, false // Stack is empty
		}
		newHead := (*Node)(oldHead).next
		if atomic.CompareAndSwapPointer(&s.head, oldHead, unsafe.Pointer(newHead)) {
			return (*Node)(oldHead).value, true
		}
	}
}

// TODO: Elimination-Backoff Stack

// TODO: Semantic Relaxation and Elastic Designs
// highly concurrent systems where minor deviations from strict LIFO order are acceptable in exchange for significant performance gains

// TODO Semantically Relaxed Stack

// TODO Wait-Free Stack: Goel Stack

// TODO Wait-Free Stack: SIM Stack

// Treiber Stack
type node[T any] struct {
	val  T
	next *node[T]
}
type treiberStack[T any] struct {
	head atomic.Pointer[node[T]] // node[T]
}

func NewTreiberStack() *treiberStack[net.Conn] {
	return &treiberStack[net.Conn]{}
}

func (s *treiberStack[T]) Push(data T) {
	newHead := node[T]{
		val:  data,
		next: s.head.Load(),
	}
	if s.head.CompareAndSwap(newHead.next, &newHead) {
		return
	} else {
		s.Push(data)
	}
}

func (s *treiberStack[T]) Pop() *T {
	oldHead := s.head.Load() // maybe panic
	if oldHead == nil {
		return nil
	}
	if s.head.CompareAndSwap(oldHead, oldHead.next) {
		return &oldHead.val
	} else {
		return s.Pop()
	}
}

// https://en.wikipedia.org/wiki/Treiber_stack
/*
   class LockFreeStack<T> {
    private class Node<T>(val value: T, val next: Node<T>?)

    private val head = AtomicReference<Node<T>?>(null)

    tailrec fun push(value: T) {
        val newHead = Node(value = value, next = head.get())
        if (head.compareAndSet(newHead.next, newHead)) return else push(value)
    }

    tailrec fun pop(): T {
        val oldHead = head.get() ?: throw NoSuchElementException("Stack is empty")
        return if (head.compareAndSet(oldHead, oldHead.next)) oldHead.value else pop()
    }
    }
*/
