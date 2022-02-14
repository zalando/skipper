package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/circuit"
	"github.com/zalando/skipper/filters/builtin"
)

func TestBreakerLeak(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(599)
		w.Write([]byte("backend"))
	}))
	defer backend.Close()

	doc := fmt.Sprintf(`main: Path("/leak") -> consecutiveBreaker(1) -> teeLoopback("tag") -> "%s"; shadow: Path("/leak") && Tee("tag") -> <shunt>;`, backend.URL)
	fr := builtin.MakeRegistry()
	settings := []circuit.BreakerSettings{{
		Type:     circuit.ConsecutiveFailures,
		Failures: 1,
	}}
	params := Params{
		Flags:                FlagsNone,
		CloseIdleConnsPeriod: -1,
		CircuitBreakers:      circuit.NewRegistry(settings...),
	}

	tp, err := newTestProxyWithFiltersAndParams(fr, doc, params, nil)
	if err != nil {
		t.Error(err)
		return
	}
	defer tp.close()

	r1, _ := http.NewRequest("POST", "http://www.example.org/leak", bytes.NewBufferString("fpp\n"))
	w1 := httptest.NewRecorder()
	tp.proxy.ServeHTTP(w1, r1)
	if w1.Code != 599 {
		t.Fatalf("first request wrong status: %d", w1.Code)
	}

	buf := bytes.NewBufferString("fpp\n")
	r1, _ = http.NewRequest("POST", "http://www.example.org/leak", buf)
	w1 = httptest.NewRecorder()
	tp.proxy.ServeHTTP(w1, r1)
	if w1.Code != 503 {
		t.Fatalf("second request wrong status: %d", w1.Code)
	}
	_, err = buf.ReadByte()
	if err != io.EOF {
		t.Errorf("request body was not read: %v", err)
	}
}
