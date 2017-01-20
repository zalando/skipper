package flowid

import (
	"strconv"
	"testing"
)

func TestFlowIdInvalidLength(t *testing.T) {
	for _, length := range []int{0, 7, 69, 100} {
		t.Run(strconv.Itoa(length), func(t *testing.T) {
			_, err := NewStandardGenerator(length)
			if err == nil {
				t.Error("request for an invalid flow id length (0) succeeded and it shouldn't")
			}
		})
	}
}

func TestFlowIdLength(t *testing.T) {
	t.Parallel()
	for expected := MinLength; expected <= MaxLength; expected++ {
		t.Run(strconv.Itoa(expected), func(t *testing.T) {
			g, err := NewStandardGenerator(expected)
			if err != nil {
				t.Errorf("failed to create standard generator: %v", err)
			} else {
				id := g.MustGenerate()
				l := len(id)
				if l != expected {
					t.Errorf("got wrong flowId len. requested %d, got %d (%s)", expected, l, id)
				}
				if !g.IsValid(id) {
					t.Errorf("generated flow id was not considered valid - %q", id)
				}
			}
		})
	}
}

func TestDeprecatedNewFlowID(t *testing.T) {
	t.Parallel()
	for expected := MinLength; expected <= MaxLength; expected++ {
		t.Run(strconv.Itoa(expected), func(t *testing.T) {
			id, err := NewFlowId(expected)
			if err != nil {
				t.Errorf("failed to generate flowid using the standard generator: %v", err)
			} else {
				l := len(id)
				if l != expected {
					t.Errorf("got wrong flowId len. Requested %d, got %d (%s)", expected, l, id)
				}

			}
		})
	}
	_, err := NewFlowId(0)
	if err == nil {
		t.Error("request for an invalid flow id length (0) succeeded and it shouldn't")
	}
}

func BenchmarkFlowIdStandardGenerator(b *testing.B) {
	for _, l := range []int{8, 10, 12, 14, 16, 26, 32, 64} {
		b.Run(strconv.Itoa(l), func(b *testing.B) {
			b.ResetTimer()
			gen, _ := NewStandardGenerator(l)
			for i := 0; i < b.N; i++ {
				gen.MustGenerate()
			}
		})
	}
}

func BenchmarkFlowIdStandardGeneratorInParallel(b *testing.B) {
	for _, l := range []int{8, 10, 12, 14, 16, 26, 32, 64} {
		b.Run(strconv.Itoa(l), func(b *testing.B) {
			gen, _ := NewStandardGenerator(l)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					gen.MustGenerate()
				}
			})
		})
	}
}
