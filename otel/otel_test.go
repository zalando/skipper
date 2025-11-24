package otel

import (
	"context"
	"testing"
)

func TestOtel(t *testing.T) {
	shutdown, err := Init(context.Background(), &Options{})
	if err != nil {
		t.Fatalf("Failied to init OTel: %v", err)
	}

	err = shutdown(context.Background())
	if err != nil {
		t.Fatalf("Failied to shutdown OTel: %v", err)
	}

}
