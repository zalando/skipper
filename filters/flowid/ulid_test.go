package flowid

import (
	"fmt"
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

	if !g.IsValid(id) {
		t.Errorf("generated flow id was not considered valid - %q", id)
	}
}

func TestInvalidFlowIDs(t *testing.T) {
	g := NewULIDGenerator()
	for _, test := range []string{
		"",
		"12345",
		"0123456789ABCDEFGHJKMNPQRSTVWXYZ",
		"01B6Y80KHY4XS20161302R3VCI",
		"01B6Y80KHY4XS20161302R3VCL",
		"01B6Y80KHY4XS20161302R3VCO",
		"01B6Y80KHY4XS20161302R3VCU",
	} {
		t.Run(fmt.Sprintf("%v", test), func(t *testing.T) {
			t.Parallel()
			if g.IsValid(test) {
				t.Errorf("invalid input was considered valid %q", test)
			}
		})
	}
}

type brokenReader int

func (r *brokenReader) Read(p []byte) (int, error) {
	return 0, io.ErrNoProgress
}

func TestBuiltInGeneratorBrokenEntropyProvider(t *testing.T) {
	g := NewULIDGeneratorWithEntropyProvider(new(brokenReader))
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

func BenchmarkFlowIdULIDGeneratorInParallel(b *testing.B) {
	gen := NewULIDGenerator()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			gen.MustGenerate()
		}
	})
}
