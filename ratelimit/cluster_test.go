package ratelimit

import (
	"math/rand"
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

	sw1, err := newFakeSwarm("n1", 5*time.Second)
	if err != nil {
		t.Errorf("Failed to start swarm1: %v", err)
	}
	defer sw1.Leave()

	crl1sw1 := NewClusterRateLimiter(s, sw1, "cr1")
	crl2sw1 := NewClusterRateLimiter(s, sw1, "cr2")
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

var fakeRand *rand.Rand = rand.New(rand.NewSource(23))

func newFakeSwarm(nodeName string, leaveTimeout time.Duration) (*swarm.Swarm, error) {
	// create port >= 1025 and < 40000
	port := uint16((fakeRand.Int() % (40000 - 1025)) + 1025)
	return swarm.NewSwarm(swarm.Options{FakeSwarm: true, FakeSwarmLocalNode: nodeName, LeaveTimeout: leaveTimeout, MaxMessageBuffer: 1024, Errors: make(chan<- error), SwarmPort: port})
}

func Test_calculateShareKnowlege(t *testing.T) {
	log.SetLevel(log.InfoLevel)
	s := Settings{
		Type:       ClusterRatelimit,
		MaxHits:    3,
		TimeWindow: 1 * time.Second,
	}
	sw1, err := newFakeSwarm("n1", 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to start swarm1: %v", err)
	}
	defer sw1.Leave()

	crl1sw1 := NewClusterRateLimiter(s, sw1, "cr1")
	defer crl1sw1.Close()

	now := time.Now()

	for _, ti := range []struct {
		name        string
		swarmValues map[string]interface{}
		epsilon     float64
		expected    float64
	}{{
		swarmValues: map[string]interface{}{
			"n1": int64(now.Sub(now.Add(-2 * time.Second))),
			"n2": int64(0),
		},
		expected: 10.0,
		epsilon:  0.1,
	}, {
		swarmValues: map[string]interface{}{
			"n1": int64(now.Sub(now.Add(-2 * time.Second))),
			"n2": int64(now.Sub(now.Add(-2 * time.Second))),
		},
		expected: 20.0,
		epsilon:  0.1,
	}} {
		t.Run(ti.name, func(t *testing.T) {

			rate := crl1sw1.calculateSharedKnowledge(ti.swarmValues)
			if !((ti.expected-ti.epsilon) <= rate && rate <= (ti.expected+ti.epsilon)) {
				t.Errorf("Failed to calculateSharedKnowledge: rate=%v expected=%v", rate, ti.expected)
			}
		})

	}
}

func TestTwoSwarms(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	s := Settings{
		Type:       ClusterRatelimit,
		MaxHits:    3,
		TimeWindow: 1 * time.Second,
	}

	sw1, err := newFakeSwarm("n1", 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to start swarm1: %v", err)
	}
	sw2, err := newFakeSwarm("n2", 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to start swarm2: %v", err)
	}

	log.Infof("sw1.Local(): %s", sw1.Local())
	log.Infof("sw2.Local(): %s", sw2.Local())
	defer sw1.Leave()
	defer sw2.Leave()

	crl1sw1 := NewClusterRateLimiter(s, sw1, "cr1")
	defer crl1sw1.Close()
	crl1sw2 := NewClusterRateLimiter(s, sw2, "cr2")
	defer crl1sw2.Close()
	backend1 := "backend1"
	//backend2 := "backend2"
	waitClean := func() {
		time.Sleep(s.TimeWindow)
	}

	t.Run("two swarm peers, single ratelimit", func(t *testing.T) {
		if !crl1sw1.Allow(backend1) {
			t.Errorf("1 %s not allowed but should", backend1)
		}

		time.Sleep(100 * time.Millisecond)
		if !crl1sw2.Allow(backend1) {
			t.Errorf("2.1 %s not allowed but should", backend1)
		}

		time.Sleep(100 * time.Millisecond)
		if !crl1sw2.Allow(backend1) {
			t.Errorf("2.2 %s not allowed but should", backend1)
		}

		time.Sleep(100 * time.Millisecond)
		if !crl1sw1.Allow(backend1) {
			t.Errorf("2 %s not allowed but should", backend1)
		}

		time.Sleep(100 * time.Millisecond)
		if crl1sw1.Allow(backend1) {
			t.Errorf("3 %s allowed but should not", backend1)
		}

		time.Sleep(100 * time.Millisecond)
		if crl1sw1.Allow(backend1) {
			t.Errorf("4 %s allowed but should not", backend1)
		}

		time.Sleep(500 * time.Millisecond)
		if crl1sw2.Allow(backend1) {
			t.Errorf("2.3 %s allowed but should not", backend1)
		}
		waitClean()
	})
}
