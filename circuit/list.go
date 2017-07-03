package circuit

// a simple list to keep breakers sorted by access order.
// It can return the first consecutive items that match a
// condition (e.g. the ones that were inactive for a
// while).
type list struct {
	first, last *Breaker
}

func (l *list) remove(from, to *Breaker) {
	if from == nil || l.first == nil {
		return
	}

	if from == l.first {
		l.first = to.next
	} else if from.prev != nil {
		from.prev.next = to.next
	}

	if to == l.last {
		l.last = from.prev
	} else if to.next != nil {
		to.next.prev = from.prev
	}

	from.prev = nil
	to.next = nil
}

func (l *list) append(from, to *Breaker) {
	if from == nil {
		return
	}

	if l.last == nil {
		l.first = from
		l.last = to
		return
	}

	l.last.next = from
	from.prev = l.last
	l.last = to
}

// appends an item to the end of the list. If the list already
// contains the item, moves it to the end.
func (l *list) appendLast(b *Breaker) {
	l.remove(b, b)
	l.append(b, b)
}

// returns the first consecutive items that match the predicate
func (l *list) getMatchingHead(predicate func(*Breaker) bool) (first, last *Breaker) {
	current := l.first
	for {
		if current == nil || !predicate(current) {
			return
		}

		if first == nil {
			first = current
		}

		last, current = current, current.next
	}
}

// takes the first consecutive items that match a predicate,
// removes them from the list, and returns them.
func (l *list) dropHeadIf(predicate func(*Breaker) bool) (from, to *Breaker) {
	from, to = l.getMatchingHead(predicate)
	l.remove(from, to)
	return
}
