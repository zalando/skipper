package eskip

import (
	"encoding/json"
	"testing"
)

func TestFilterUnmarshalJSON(t *testing.T) {
	f := &Filter{}
	json.Unmarshal([]byte(`{"name": "yolo", "args": ["hello"]}`), f)
	t.Log(f)
}
