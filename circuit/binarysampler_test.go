package circuit

import "testing"

func TestBinarySampler(t *testing.T) {
	expectCount := func(t *testing.T, s *binarySampler, c int) {
		if s.count != c {
			t.Errorf("unexpected count, got: %d, expected: %d", s.count, c)
		}
	}

	t.Run("wrong init arg defaults to 1", func(t *testing.T) {
		s := newBinarySampler(-3)
		expectCount(t, s, 0)
		s.tick(true)
		expectCount(t, s, 1)
		s.tick(true)
		expectCount(t, s, 1)
	})

	t.Run("returns right count when not filled", func(t *testing.T) {
		s := newBinarySampler(6)
		s.tick(true)
		s.tick(false)
		s.tick(true)
		expectCount(t, s, 2)
	})

	t.Run("returns right count after filled", func(t *testing.T) {
		s := newBinarySampler(3)
		s.tick(false)
		s.tick(true)
		s.tick(false)
		s.tick(true)
		expectCount(t, s, 2)
	})

	t.Run("shifts the reservoir when filled", func(t *testing.T) {
		s := newBinarySampler(3)
		s.tick(true)
		s.tick(false)
		s.tick(true)
		s.tick(false)
		expectCount(t, s, 1)
	})

	t.Run("shifts through multiple frames", func(t *testing.T) {
		const size = 314
		s := newBinarySampler(size)

		for range size + size/2 {
			s.tick(true)
		}

		expectCount(t, s, size)
	})

	t.Run("uses the right 'amount of memory'", func(t *testing.T) {
		const size = 314
		s := newBinarySampler(size)
		for range size + size/2 {
			s.tick(true)
		}

		expectedFrames := size / 64
		if size%64 > 0 {
			expectedFrames++
		}

		if len(s.frames) != expectedFrames {
			t.Errorf(
				"unexpected number of frames, got: %d, expected: %d",
				len(s.frames), expectedFrames,
			)
		}
	})
}
