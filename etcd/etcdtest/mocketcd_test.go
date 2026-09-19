package etcdtest

import "testing"

func TestMockETCD(t *testing.T) {

	err := Start()
	if err != nil {
		t.Fatalf("Failed to start mocketcd: %v", err)
	}

	key := "testdataKey"
	val := "test data"
	PutData(key, val)

	dat, err := GetNode(key)
	if err != nil {
		t.Fatalf("Failed to get data: %v", err)
	}
	if dat != val {
		t.Fatalf("Failed to get the same data as we put %q, got %q", val, dat)
	}

	DeleteData(key)
	dat, err = GetNode(key)
	if err != nil {
		t.Fatalf("Failed to get data: %v", err)
	}
	if dat != "" {
		t.Fatalf("failes to delete data: %q", dat)
	}

	PutData(key, val)
	ResetData()
	dat, err = GetNode(key)
	if err != nil {
		t.Fatalf("Failed to get data: %v", err)
	}
	if dat != "" {
		t.Fatalf("failes to reset data: %q", dat)
	}

	DeleteAll()

	err = Stop()
	if err != nil {
		t.Fatalf("Failed to stop mocketcd: %v", err)
	}
}
