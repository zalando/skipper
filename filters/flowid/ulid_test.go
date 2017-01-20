package flowid

import (
	"io"
	"testing"
)

func TestUlidGenerator(t *testing.T) {
	g := NewULIDGenerator()
	id, err := g.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Error("generated flowid is empty")
	}

	id = g.MustGenerate()
	if id == "" {
		t.Error("generated flowid is empty")
	}
}

type brokenReader int

func (r *brokenReader) Read(p []byte) (int, error) {
	return 0, io.ErrNoProgress
}

func TestBuiltInGeneratorBrokenEntropyProvider(t *testing.T) {
	g := NewULIDGeneratorWithEntropy(new(brokenReader))
	_, err := g.Generate()
	if err == nil {
		t.Fatal("expected an error from the entropy provider bur err is nil")
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected an error from the entropy provider bur err is nil")
		}
	}()
	g.MustGenerate()
}

func BenchmarkFlowIdULIDGenerator(b *testing.B) {
	b.Run("Std", func(b *testing.B) {
		gen := NewULIDGenerator()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			gen.MustGenerate()
		}
	})
}
