package flowid

import (
	"strconv"
	"testing"
)

func TestFlowIdInvalidLength(t *testing.T) {
	_, err := NewFlowId(0)
	if err == nil {
		t.Error("Request for an invalid flow id length (0) succeeded and it shouldn't")
	}

	_, err = NewFlowId(100)
	if err == nil {
		t.Error("Request for an invalid flow id length (100) succeeded and it shouldn't")
	}
}

func TestFlowIdLength(t *testing.T) {
	for expected := MinLength; expected <= MaxLength; expected++ {
		t.Run(strconv.Itoa(expected), func(t *testing.T) {
			g, err := newBuiltInGenerator(expected)
			if err != nil {
				t.Errorf("failed to create built-in generator: %v", err)
			} else {
				id := g.MustGenerate()
				l := len(id)
				if l != expected {
					t.Errorf("got wrong flowId len. Requested %d, got %d (%s)", expected, l, id)
				}

			}
		})
	}
}

func TestDeprecatedNewFlowID(t *testing.T) {
	for expected := MinLength; expected <= MaxLength; expected++ {
		t.Run(strconv.Itoa(expected), func(t *testing.T) {
			id, err := NewFlowId(expected)
			if err != nil {
				t.Errorf("failed to generate flowid using the built-in generator: %v", err)
			} else {
				l := len(id)
				if l != expected {
					t.Errorf("got wrong flowId len. Requested %d, got %d (%s)", expected, l, id)
				}

			}
		})
	}
}

func BenchmarkFlowIdBuiltInGenerator(b *testing.B) {
	for _, l := range []int{8, 10, 12, 14, 16, 26, 32, 64} {
		b.Run(strconv.Itoa(l), func(b *testing.B) {
			b.ResetTimer()
			gen, _ := newBuiltInGenerator(l)
			for i := 0; i < b.N; i++ {
				gen.MustGenerate()
			}
		})
	}
}
