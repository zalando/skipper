package ratelimit

import (
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/swarm"
)

func TestSingleSwarm(t *testing.T) {
	s := Settings{
		Type:       ClusterRatelimit,
		MaxHits:    3,
		TimeWindow: 1 * time.Second,
	}

	sw1, err := swarm.NewSwarm(swarm.Options{FakeSwarm: true, LeaveTimeout: 5 * time.Second, MaxMessageBuffer: 1024, Errors: make(chan<- error), SwarmPort: 10000})
	if err != nil {
		t.Errorf("Failed to start swarm1: %v", err)
	}
	defer sw1.Leave()

	crl1sw1 := NewClusterRateLimiter(s, sw1)
	crl2sw1 := NewClusterRateLimiter(s, sw1)
	backend1 := "foo"
	backend2 := "bar"
	waitClean := func() {
		time.Sleep(s.TimeWindow)
	}

	t.Run("single swarm peer, single ratelimit", func(t *testing.T) {
		if !crl1sw1.Allow(backend1) {
			t.Errorf("%s not allowed but should", backend1)
		}
		if !crl1sw1.Allow(backend1) {
			t.Errorf("%s not allowed but should", backend1)
		}
		if crl1sw1.Allow(backend1) {
			t.Errorf("%s allowed but should not", backend1)
		}
		if crl1sw1.Allow(backend2) {
			t.Errorf("%s not allowed but should", backend2)
		}
		waitClean()
		if !crl1sw1.Allow(backend2) {
			t.Errorf("after wait clean %s not allowed but should", backend2)
		}
		if !crl1sw1.Allow(backend1) {
			t.Errorf("after wait clean %s not allowed but should", backend1)
		}
		if crl1sw1.Allow(backend1) {
			t.Errorf("%s allowed but should not", backend1)
		}
		if crl1sw1.Allow(backend2) {
			t.Errorf("%s allowed but should not", backend2)
		}
		waitClean()
	})

	t.Run("single swarm peer, multiple ratelimiters", func(t *testing.T) {
		if !crl1sw1.Allow(backend1) {
			t.Errorf("%s not allowed but should", backend1)
		}
		if !crl1sw1.Allow(backend2) {
			t.Errorf("%s not allowed but should", backend2)
		}
		if !crl2sw1.Allow(backend1) {
			t.Errorf("%s not allowed but should", backend1)
		}
		if !crl2sw1.Allow(backend2) {
			t.Errorf("%s not allowed but should", backend2)
		}
		if crl1sw1.Allow(backend1) {
			t.Errorf("%s allowed but should not", backend1)
		}
		if crl2sw1.Allow(backend2) {
			t.Errorf("%s not allowed but should", backend2)
		}
		waitClean()
	})

}

func TestTwoSwarms(t *testing.T) {
	s := Settings{
		Type:       ClusterRatelimit,
		MaxHits:    3,
		TimeWindow: 1 * time.Second,
	}

	sw1, err := swarm.NewSwarm(swarm.Options{FakeSwarm: true, LeaveTimeout: 5 * time.Second, MaxMessageBuffer: 1024, Errors: make(chan<- error), SwarmPort: 10000})
	if err != nil {
		t.Fatalf("Failed to start swarm1: %v", err)
	}
	sw2, err := swarm.NewSwarm(swarm.Options{FakeSwarm: true, LeaveTimeout: 5 * time.Second, MaxMessageBuffer: 1024, Errors: make(chan<- error), SwarmPort: 10001})
	if err != nil {
		t.Fatalf("Failed to start swarm2: %v", err)
	}

	log.Infof("sw1.Local(): %s", sw1.Local())
	log.Infof("sw2.Local(): %s", sw2.Local())
	defer sw1.Leave()
	defer sw2.Leave()

	crl1sw1 := NewClusterRateLimiter(s, sw1)
	defer crl1sw1.Close()
	crl1sw2 := NewClusterRateLimiter(s, sw2)
	defer crl1sw2.Close()
	backend1 := "backend1"
	//backend2 := "backend2"
	waitClean := func() {
		time.Sleep(s.TimeWindow)
	}

	t.Run("two swarm peers, single ratelimit", func(t *testing.T) {
		if !crl1sw1.Allow(backend1) {
			t.Errorf("%s not allowed but should", backend1)
		}
		if !crl1sw2.Allow(backend1) {
			t.Errorf("%s not allowed but should", backend1)
		}
		time.Sleep(500 * time.Millisecond)
		if crl1sw2.Allow(backend1) {
			t.Errorf("%s allowed but should not", backend1)
		}
		// if crl1sw1.Allow(backend2) {
		// 	t.Errorf("%s allowed but should not", backend2)
		// }
		waitClean()
	})
}
