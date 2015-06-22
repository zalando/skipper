package mock

import "testing"

func TestStartingEtcdMultipleTimes(t *testing.T) {
	err := Etcd()
	if err != nil {
		t.Error("shouldn't return error")
	}

	err = Etcd()
	if err != nil {
		t.Error("shouldn't return error")
	}
}
