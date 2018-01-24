package loadbalancer

import (
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
)

var testlb *LB

func createNewLB() *LB {
	if testlb == nil {
		testlb = &LB{
			stop:                false,
			healthcheckInterval: 30 * time.Second,
		}
	}
	return testlb
}

func TestNewLB(t *testing.T) {
	tests := []struct {
		name                string
		healthcheckInterval time.Duration
		want                *LB
	}{
		{
			name:                "return nil if healthcheckInterval is 0",
			healthcheckInterval: 0,
			want:                nil,
		}, {
			name:                "no run of goroutine, because long duration",
			healthcheckInterval: 30 * time.Second,
			want:                createNewLB(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewLB(tt.healthcheckInterval); got != tt.want && !(got.healthcheckInterval == tt.want.healthcheckInterval || got.stop == tt.want.stop) {
				t.Errorf("NewLB() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLB_AddHealthcheck(t *testing.T) {
	tests := []struct {
		name    string
		lb      *LB
		backend string
		want    state
	}{
		{
			name:    "nil lb should not panic",
			lb:      nil,
			backend: "http://www.example.com/",
			want:    unhealthy,
		},
		{
			name:    "add backend to health check",
			lb:      NewLB(3 * time.Minute),
			backend: "http://www.example.com/",
			want:    unhealthy,
		},
		{
			name:    "add backend to health check and do health check",
			lb:      NewLB(250 * time.Millisecond),
			backend: "http://127.0.0.1:1333/",
			want:    dead,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.lb.AddHealthcheck(tt.backend)
			if tt.lb != nil {
				time.Sleep(1 * time.Second) // within 1s healthchecks should have a state from populateChecks()
				tt.lb.RLock()
				st, ok := tt.lb.routeState[tt.backend]
				tt.lb.RUnlock()
				if !ok || st != tt.want {
					t.Errorf("backend %s is %v, expected %v", tt.backend, st, tt.want)
				}
			}
		})
		// cleanup
		if tt.lb != nil {
			tt.lb.sigtermSignal <- syscall.SIGTERM
		}
	}
}

func createRoute(id, backend, group string) *eskip.Route {
	return &eskip.Route{
		Id:      id,
		Backend: backend,
		Group:   group,
	}
}

func TestLB_FilterHealthyMemberRoutes(t *testing.T) {
	tests := []struct {
		name       string
		lb         *LB
		routes     []*eskip.Route
		routeState map[string]state
		want       []*eskip.Route
	}{
		{
			name:   "nil lb should not filter routes",
			lb:     nil,
			routes: []*eskip.Route{createRoute("id", "http://127.0.0.1:1234/", "g1")},
			want:   []*eskip.Route{createRoute("id", "http://127.0.0.1:1234/", "g1")},
		},
		{
			name:   "one non filtered route",
			lb:     NewLB(750 * time.Millisecond),
			routes: []*eskip.Route{createRoute("id", "http://example.com/", "foo")},
			want:   []*eskip.Route{createRoute("id", "http://example.com/", "foo")},
		},
		{
			name:   "one filtered route",
			lb:     NewLB(750 * time.Millisecond),
			routes: []*eskip.Route{createRoute("id", "http://127.0.0.1:1334/", "bar")},
			want:   []*eskip.Route{},
		},
		{
			name: "multiple routes two filtered route",
			lb:   NewLB(750 * time.Millisecond),
			routes: []*eskip.Route{
				createRoute("id", "http://example.com/", "foo"),
				createRoute("id", "http://127.0.0.1:1334/", "baz"),
				createRoute("id", "http://127.0.0.1:1335/", "baz"),
			},
			want: []*eskip.Route{createRoute("id", "http://example.com/", "foo")},
		},
	}
	for _, tt := range tests {
		for _, r := range tt.routes {
			tt.lb.AddHealthcheck(r.Backend)
		}
		time.Sleep(1 * time.Second)

		t.Run(tt.name, func(t *testing.T) {
			if got := tt.lb.FilterHealthyMemberRoutes(tt.routes); !cmp.Equal(got, tt.want) {
				t.Errorf("%s, got: %v, expected: %v", tt.name, got, tt.want)
				log.Error(cmp.Diff(got, tt.want))
			}
		})
		// cleanup
		if tt.lb != nil {
			tt.lb.sigtermSignal <- syscall.SIGTERM
		}
	}
}
