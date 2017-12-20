package swarm

import (
	"net/http"
	"net/http/httptest"
	"testing"
	// "github.com/sanity-io/litter"
)

func TestGetKubeNodeInfo(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(content))
	}))

	c := NewNodeInfoClient(s.URL)
	_, err := c.GetNodeInfo("foo", "bar")
	if err != nil {
		t.Error(err)
		return
	}

	// for i := range n {
	// litter.Dump(n[i])
	// }
}
