package circuit

import "testing"

func checkListDirection(t *testing.T, first *Breaker, reverse bool, expected ...*Breaker) {
}

func checkList(t *testing.T, first *Breaker, expected ...*Breaker) {
	current := first
	for i, expected := range expected {
		if current == nil {
			t.Error("less items in the list than expected")
			return
		}

		if i == 0 && current.prev != nil {
			t.Error("damaged list")
			return
		}

		if expected != current {
			t.Error("invalid order")
			return
		}

		current = current.next
	}

	if current != nil {
		t.Error("more items in the list than expected")
	}
}

func appendAll(l *list, items ...*Breaker) {
	for _, i := range items {
		l.appendLast(i)
	}
}

func TestListAppend(t *testing.T) {
	t.Run("append", func(t *testing.T) {
		l := &list{}

		b0 := newBreaker(BreakerSettings{})
		b1 := newBreaker(BreakerSettings{})
		b2 := newBreaker(BreakerSettings{})
		appendAll(l, b0, b1, b2)
		checkList(t, l.first, b0, b1, b2)
	})

	t.Run("reappend", func(t *testing.T) {
		l := &list{}

		b0 := newBreaker(BreakerSettings{})
		b1 := newBreaker(BreakerSettings{})
		b2 := newBreaker(BreakerSettings{})
		appendAll(l, b0, b1, b2)

		l.appendLast(b1)

		checkList(t, l.first, b0, b2, b1)
	})

	t.Run("reappend first", func(t *testing.T) {
		l := &list{}

		b0 := newBreaker(BreakerSettings{})
		b1 := newBreaker(BreakerSettings{})
		b2 := newBreaker(BreakerSettings{})
		appendAll(l, b0, b1, b2)

		l.appendLast(b0)

		checkList(t, l.first, b1, b2, b0)
	})

	t.Run("reappend last", func(t *testing.T) {
		l := &list{}

		b0 := newBreaker(BreakerSettings{})
		b1 := newBreaker(BreakerSettings{})
		b2 := newBreaker(BreakerSettings{})
		appendAll(l, b0, b1, b2)

		l.appendLast(b2)

		checkList(t, l.first, b0, b1, b2)
	})
}

func TestDropHead(t *testing.T) {
	createToDrop := func() *Breaker { return newBreaker(BreakerSettings{Host: "drop"}) }
	createNotToDrop := func() *Breaker { return newBreaker(BreakerSettings{Host: "no-drop"}) }
	predicate := func(item *Breaker) bool { return item.settings.Host == "drop" }

	t.Run("from empty", func(t *testing.T) {
		l := &list{}
		drop, _ := l.dropHeadIf(predicate)
		checkList(t, l.first)
		checkList(t, drop)
	})

	t.Run("drop matching", func(t *testing.T) {
		l := &list{}

		b0 := createToDrop()
		b1 := createToDrop()
		b2 := createNotToDrop()
		b3 := createToDrop()
		b4 := createNotToDrop()
		appendAll(l, b0, b1, b2, b3, b4)

		drop, _ := l.dropHeadIf(predicate)
		checkList(t, l.first, b2, b3, b4)
		checkList(t, drop, b0, b1)
	})

	t.Run("none match", func(t *testing.T) {
		l := &list{}

		b0 := createNotToDrop()
		b1 := createToDrop()
		b2 := createNotToDrop()
		b3 := createToDrop()
		b4 := createNotToDrop()
		appendAll(l, b0, b1, b2, b3, b4)

		drop, _ := l.dropHeadIf(predicate)
		checkList(t, l.first, b0, b1, b2, b3, b4)
		checkList(t, drop)
	})

	t.Run("all match", func(t *testing.T) {
		l := &list{}

		b0 := createToDrop()
		b1 := createToDrop()
		b2 := createToDrop()
		b3 := createToDrop()
		b4 := createToDrop()
		appendAll(l, b0, b1, b2, b3, b4)

		drop, _ := l.dropHeadIf(predicate)
		checkList(t, l.first)
		checkList(t, drop, b0, b1, b2, b3, b4)
	})
}
