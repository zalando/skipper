package primitive

import (
	"net/http"
	"testing"
)

func TestFalse(t *testing.T) {
	f := NewFalse()
	req, _ := http.NewRequest("GET", "http://false.test", nil)
	p, err := f.Create(nil)
	if err != nil {
		t.Fatalf("Failed to create: %v", err)
	}
	if p.Match(req) {
		t.Fatal("False() should never match")
	}
	if f.Name() == "" {
		t.Fatal("predicate should have a name")
	}
}

func TestTrue(t *testing.T) {
	tr := NewTrue()
	req, _ := http.NewRequest("GET", "http://true.test", nil)
	p, err := tr.Create(nil)
	if err != nil {
		t.Fatalf("Failed to create: %v", err)
	}
	if !p.Match(req) {
		t.Fatal("True() should always match")
	}
	if tr.Name() == "" {
		t.Fatal("predicate should have a name")
	}
}
