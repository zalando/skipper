package etcd

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTimedOutEndpointsRotation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))

	s.Close()

	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(5 * time.Millisecond))
	}))
	defer s2.Close()

	c, err := New(Options{Endpoints: []string{s.URL, s2.URL, "neverreached"}, Prefix: "/skippertest-invalid"})
	c.client.Timeout = time.Duration(1 * time.Millisecond)

	if err != nil {
		t.Error(err)
		return
	}

	_, err = c.LoadAll()

	if err == nil {
		t.Error("failed to fail")
	}

	nerr, ok := err.(net.Error)

	if !ok || !nerr.Timeout() {
		t.Error("timeout error expected")
	}

	expectedEndpoints := []string{s2.URL, "neverreached", s.URL}

	if strings.Join(c.endpoints, ";") != strings.Join(expectedEndpoints, ";") {
		t.Error("wrong endpoints rotation")
	}
}
