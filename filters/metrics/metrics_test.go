package metrics

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/diag"
	"github.com/zalando/skipper/metrics/metricstest"
	"github.com/zalando/skipper/proxy"
	"github.com/zalando/skipper/proxy/proxytest"
)

func TestCreateTimer(t *testing.T) {
	for _, test := range []struct {
		name string
		args []interface{}
		fail bool
	}{{
		name: "no args",
		fail: true,
	}, {
		name: "too many args",
		args: []interface{}{"foo", "bar"},
		fail: true,
	}, {
		name: "not string",
		args: []interface{}{42},
		fail: true,
	}, {
		name: "success",
		args: []interface{}{"foo"},
	}} {
		t.Run(test.name, func(t *testing.T) {
			spec := NewTimer()
			_, err := spec.CreateFilter(test.args)
			if test.fail && err == nil {
				t.Fatal("failed to fail")
			}

			if !test.fail && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTimer(t *testing.T) {
	fr := make(filters.Registry)
	fr.Register(NewTimer())
	fr.Register(diag.NewLatency())

	const inputKey = "foo"
	route := fmt.Sprintf(`* -> timer("%s") -> latency(1) -> <shunt>`, inputKey)
	r, err := eskip.Parse(route)
	if err != nil {
		t.Fatal(err)
	}

	m := &metricstest.MockMetrics{}
	p := proxytest.WithParams(fr, proxy.Params{Metrics: m}, r...)
	defer p.Close()

	rsp, err := http.Get(p.URL)
	if err != nil {
		t.Fatal(err)
	}

	rsp.Body.Close()

	key := fmt.Sprintf("timer.custom.%s", inputKey)
	d, _ := m.Timer(key)
	if len(d) == 0 {
		t.Fatal("failed to collect metrics with the key")
	}

	if d[0] < time.Millisecond {
		t.Fatal("failed to measure the right duration", d[0])
	}
}
