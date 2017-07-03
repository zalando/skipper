package circuit

// contains a series of events with 0 or 1 values, e.g. errors or successes,
// within a limited window.
// count contains the number of events with the value of 1 in the window.
// compresses the event storage by 64.
type binarySampler struct {
	size   int
	filled int
	frames []uint64
	pad    uint64
	count  int
}

func newBinarySampler(size int) *binarySampler {
	if size <= 0 {
		size = 1
	}

	return &binarySampler{
		size: size,
		pad:  64 - uint64(size)%64,
	}
}

func highestSet(frame, pad uint64) bool {
	return frame&(1<<(63-pad)) != 0
}

func shift(frames []uint64) {
	highestFrame := len(frames) - 1
	for i := highestFrame; i >= 0; i-- {
		h := highestSet(frames[i], 0)
		frames[i] = frames[i] << 1
		if h && i < highestFrame {
			frames[i+1] |= 1
		}
	}
}

func (s *binarySampler) tick(set bool) {
	filled := s.filled == s.size

	if filled && highestSet(s.frames[len(s.frames)-1], s.pad) {
		s.count--
	}

	if !filled {
		if len(s.frames) <= s.filled/64 {
			s.frames = append(s.frames, 0)
		}

		s.filled++
	}

	shift(s.frames)

	if set {
		s.count++
		s.frames[0] |= 1
	}
}
