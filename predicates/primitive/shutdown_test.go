package primitive

import (
	"net/http"
	"syscall"
	"testing"
	"time"
)

func TestShutdown(t *testing.T) {
	s, sigs := newShutdown()
	p, err := s.Create([]any{})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", "https://www.example.org", nil)

	if p.Match(req) {
		t.Error("unexpected shutdown")
	}

	sigs <- syscall.SIGTERM
	time.Sleep(100 * time.Millisecond)

	if !p.Match(req) {
		t.Error("expected shutdown")
	}

	if s.Name() == "" {
		t.Fatal("predicate should have a name")
	}
}

func TestShutdownErrors(t *testing.T) {
	s := NewShutdown()
	_, err := s.Create([]any{"wrong args"})
	if err == nil {
		t.Fatal("Failed to get err")
	}
}
