package loadbalancer

import "testing"

func TestInvalidGroupSize(t *testing.T) {
	spec := NewDecide()

	if _, err := spec.CreateFilter([]interface{}{"foo-group", 0}); err == nil {
		t.Error("failed to fail with group size 0")
	}

	if _, err := spec.CreateFilter([]interface{}{"foo-group", -3}); err == nil {
		t.Error("failed to fail with negative group size")
	}
}
