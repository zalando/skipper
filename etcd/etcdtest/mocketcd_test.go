package etcdtest

import (
	"encoding/json"
	"testing"
)

type Response struct {
	Action string `json:"action"`
	Node   Node   `json:"node"`
}

type Node struct {
	Key           string `json:"key"`
	Value         string `json:"value"`
	ModifiedIndex int64  `json:"modifiedIndex"`
	CreatedIndex  int64  `json:"createdIndex"`
}

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

	var rsp Response
	err = json.Unmarshal([]byte(dat), &rsp)
	if err != nil {
		t.Fatalf("Failed to Unmarshal: %v", err)
	}
	if rsp.Node.Value != val {
		t.Fatalf("Failed to get the same data as we put for %q, got %q", val, rsp.Node.Value)
	}

	DeleteData(key)
	dat, _ = GetNode(key)
	if dat != "" {
		t.Fatalf("failes to delete data for key %q: %q", rsp.Node.Key, dat)
	}

	PutData(key, val)
	ResetData()
	dat, _ = GetNode(rsp.Node.Key)
	if dat != "" {
		t.Fatalf("failes to reset data: %q", dat)
	}

	PutData(key, val)
	DeleteAll()
	dat, _ = GetNode(key)
	if dat != "" {
		t.Fatalf("failes to deleteall data: %q", dat)
	}

	err = Stop()
	if err != nil {
		t.Fatalf("Failed to stop mocketcd: %v", err)
	}
}
