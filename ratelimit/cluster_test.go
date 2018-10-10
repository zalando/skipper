package ratelimit

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/swarm"
)

var fakeRand *rand.Rand = rand.New(rand.NewSource(23))

func newFakeSwarm(nodeName string, leaveTimeout time.Duration) (*swarm.Swarm, error) {
	// create port >= 1025 and < 40000
	port := uint16((fakeRand.Int() % (40000 - 1025)) + 1025)
	//return swarm.NewSwarm(swarm.Options{FakeSwarm: true, FakeSwarmLocalNode: nodeName, LeaveTimeout: leaveTimeout, MaxMessageBuffer: 1024, Errors: make(chan<- error), SwarmPort: port})
	return swarm.NewSwarm(swarm.Options{FakeSwarm: true, FakeSwarmLocalNode: fmt.Sprintf("%s-%d", nodeName, port), LeaveTimeout: leaveTimeout, MaxMessageBuffer: 1024, Errors: make(chan<- error), SwarmPort: port})
}

func TestSingleSwarmSingleRatelimit(t *testing.T) {
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
	backend1 := "TestSingleSwarmSingleRatelimit backend1"

	t.Run("single swarm peer, single ratelimit", func(t *testing.T) {
		if !crl1sw1.Allow(backend1) {
			t.Errorf("1 %s not allowed but should", backend1)
		}
		time.Sleep(100 * time.Millisecond)
		println("============")

		if !crl1sw1.Allow(backend1) {
			t.Errorf("2 %s not allowed but should", backend1)
		}
		time.Sleep(100 * time.Millisecond)
		println("============")

		if !crl1sw1.Allow(backend1) {
			t.Errorf("3 %s allowed but should not", backend1)
		}

		time.Sleep(100 * time.Millisecond)
		println("============")

		if crl1sw1.Allow(backend1) {
			t.Errorf("4 %s allowed but should not", backend1)
		}
	})
}

func TestSingleSwarm(t *testing.T) {
	s := Settings{
		Type:       ClusterRatelimit,
		MaxHits:    3,
		TimeWindow: 1 * time.Second,
	}
	backend1 := "TestSingleSwarm backend1"
	backend2 := "TestSingleSwarm backend2"

	t.Run("single swarm peer, single ratelimit", func(t *testing.T) {
		sw1, err := newFakeSwarm("n1", 5*time.Second)
		if err != nil {
			t.Errorf("Failed to start swarm1: %v", err)
		}
		defer sw1.Leave()
		crl1sw1 := NewClusterRateLimiter(s, sw1, "cr1")

		for i := 1; i <= s.MaxHits; i++ {
			if !crl1sw1.Allow(backend1) {
				t.Errorf("%s not allowed but should in %d iteration", backend1, i)
			}
		}

		if crl1sw1.Allow(backend2) {
			t.Errorf("%s allowed but should not", backend2)
		}
	})

	t.Run("single swarm peer, multiple ratelimiters", func(t *testing.T) {
		sw1, err := newFakeSwarm("n1", 5*time.Second)
		if err != nil {
			t.Errorf("Failed to start swarm1: %v", err)
		}
		defer sw1.Leave()
		crl1sw1 := NewClusterRateLimiter(s, sw1, "cr1")
		crl2sw1 := NewClusterRateLimiter(s, sw1, "cr2")

		for i := 0; i < s.MaxHits; i++ {
			if !crl1sw1.Allow(backend1) {
				t.Errorf("%s not allowed but should, iteration %d", backend1, i)
			}
		}
		if crl1sw1.Allow(backend1) {
			t.Errorf("%s allowed but should not", backend1)
		}
		if !crl2sw1.Allow(backend2) {
			t.Errorf("%s not allowed but should", backend2)
		}

		// one is already tested
		for i := 1; i < s.MaxHits; i++ {
			if !crl2sw1.Allow(backend2) {
				t.Errorf("%s not allowed but should, iteration %d", backend2, i)
			}
		}
		if crl1sw1.Allow(backend1) {
			t.Errorf("%s allowed but should not", backend1)
		}
		if crl2sw1.Allow(backend2) {
			t.Errorf("%s allowed but should not", backend2)
		}
	})

}

func Test_calcTotalRequestRate_ManyHitsSmallTimeWindow(t *testing.T) {
	log.SetLevel(log.InfoLevel)
	s := Settings{
		Type:       ClusterRatelimit,
		MaxHits:    100,
		TimeWindow: 1 * time.Second,
	}
	sw1, err := newFakeSwarm("n1", 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to start swarm1: %v", err)
	}
	defer sw1.Leave()

	crl1sw1 := NewClusterRateLimiter(s, sw1, "cr1")
	defer crl1sw1.Close()

	now := time.Now().UTC().UnixNano()

	for _, ti := range []struct {
		name        string
		swarmValues map[string]interface{}
		epsilon     float64
		expected    float64
	}{{
		name: "800ms both to reach 50",
		swarmValues: map[string]interface{}{
			"n1": now - int64(800*time.Millisecond),
			"n2": now - int64(800*time.Millisecond),
		},
		// 50req in 800ms --> 62.5req/s per node, 125req/s shared state
		// global: 125req/s
		expected: 125.0,
		epsilon:  0.1,
	}, {
		name: "800ms one, other 200ms to reach 50",
		swarmValues: map[string]interface{}{
			"n1": now - int64(800*time.Millisecond),
			"n2": now - int64(200*time.Millisecond),
		},
		// 50req in 800ms --> 62.5req/s, 50req in 200ms --> 250req/s
		// global: 312.5req/s
		expected: 312.5,
		epsilon:  0.1,
	}, {
		name: "800ms one, other 3200ms to reach 50",
		swarmValues: map[string]interface{}{
			"n1": now - int64(800*time.Millisecond),
			"n2": now - int64(3200*time.Millisecond),
		},
		// 50req in 800ms --> 62.5req/s, 50req in 3200ms --> 15.625req/s
		// global: 78.125req/s
		expected: 78.125,
		epsilon:  0.1,
	}, {
		name: "3200ms one, other 800ms to reach 50",
		swarmValues: map[string]interface{}{
			"n1": now - int64(3200*time.Millisecond),
			"n2": now - int64(800*time.Millisecond),
		},
		// 50req in 800ms --> 62.5req/s, 50req in 3200ms --> 15.625req/s
		// global: 78.125req/s
		expected: 78.125,
		epsilon:  0.1,
	}} {
		t.Run(ti.name, func(t *testing.T) {
			rate := crl1sw1.calcTotalRequestRate(now, ti.swarmValues)
			if !((ti.expected-ti.epsilon) <= rate && rate <= (ti.expected+ti.epsilon)) {
				t.Errorf("Failed to calcTotalRequestRate: rate=%v expected=%v", rate, ti.expected)
			}

			// check that it times out, rate should be always below MaxHits
			rate = crl1sw1.calcTotalRequestRate(now+int64(s.TimeWindow), ti.swarmValues)
			if rate > float64(s.MaxHits) {
				t.Errorf("Failed to drop below maxhits calcTotalRequestRate: rate=%v but should be less than %v", rate, s.MaxHits)
			}
		})

	}
}

func Test_calcTotalRequestRate_LowTrafficLongTimeFrame(t *testing.T) {
	log.SetLevel(log.InfoLevel)
	s := Settings{
		Type:       ClusterRatelimit,
		MaxHits:    10,
		TimeWindow: 1 * time.Hour,
	}
	sw1, err := newFakeSwarm("n1", 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to start swarm1: %v", err)
	}
	defer sw1.Leave()

	crl1sw1 := NewClusterRateLimiter(s, sw1, "cr1")
	defer crl1sw1.Close()

	now := time.Now().UTC().UnixNano()
	for _, ti := range []struct {
		name        string
		swarmValues map[string]interface{}
		epsilon     float64
		expected    float64
	}{{
		name:     "no swarmValues",
		expected: 0,
		epsilon:  0.1,
	}, {
		name: "both have swarmValues, one has a hit, the other no hit",
		swarmValues: map[string]interface{}{
			"n1": now - int64(59*time.Minute),
			"n2": int64(0),
		},
		// 5 req in 59min --> 5.08 req/h shared state, 0req/h
		// global: 5.08req/h
		expected: 5.08,
		epsilon:  0.1,
	}, {
		name: "both have swarmValues, both have too many hits",
		swarmValues: map[string]interface{}{
			"n1": now - int64(59*time.Minute),
			"n2": now - int64(59*time.Minute),
		},
		// 2x: 5 req in 59min --> 5.08 req/h shared state
		// global: 10.16req/h
		expected: 10.16,
		epsilon:  0.1,
	}, {
		name: "both have swarmValues, one has a too many hits",
		swarmValues: map[string]interface{}{
			"n1": now - int64(20*time.Minute),
			"n2": int64(0),
		},
		// 5 req in 20min --> 15req/h shared state
		// global: 15req/h
		expected: 15.0,
		epsilon:  0.1,
	}, {
		name: "one has swarmValue the other not, one has a too many hits",
		swarmValues: map[string]interface{}{
			"n1": now - int64(20*time.Minute),
		},
		// 10 req in 20min --> 30req/h shared state
		// global: 30req/h
		expected: 30.0,
		epsilon:  0.1,
	}, {
		name: "one has swarmValue the other not, one has a ok rate",
		swarmValues: map[string]interface{}{
			"n1": now - int64(61*time.Minute),
		},
		// 10 req in 61min --> 9.83req/h shared state
		// global: 9.83req/h
		expected: 9.83,
		epsilon:  0.1,
	}, {
		name: "both have swarmValues, both have an ok rate",
		swarmValues: map[string]interface{}{
			"n1": now - int64(61*time.Minute),
			"n2": now - int64(61*time.Minute),
		},
		// 2x: 5 req in 61min --> 4.92/h shared state
		// global: 9.84req/h
		expected: 9.84,
		epsilon:  0.1,
	}, {
		name: "both have swarmValues, one has a ok rate the other not, together ok",
		swarmValues: map[string]interface{}{
			"n1": now - int64(65*time.Minute),
			"n2": now - int64(59*time.Minute),
		},
		expected: (60 * 5 / 65.0) + (60 * 5 / 59.0),
		epsilon:  0.1,
	}, {
		name: "both have swarmValues, one has a ok rate the other not, together they are not ok",
		swarmValues: map[string]interface{}{
			"n1": now - int64(65*time.Minute),
			"n2": now - int64(40*time.Minute),
		},
		expected: (60 * 5 / 65.0) + (60 * 5 / 40.0),
		epsilon:  0.1,
	}} {
		t.Run(ti.name, func(t *testing.T) {
			rate := crl1sw1.calcTotalRequestRate(now, ti.swarmValues)
			if !((ti.expected-ti.epsilon) <= rate && rate <= (ti.expected+ti.epsilon)) {
				t.Errorf("Failed to calcTotalRequestRate: rate=%v expected=%v", rate, ti.expected)
			}

			// check that it times out, rate should be always 0
			rate = crl1sw1.calcTotalRequestRate(now+int64(s.TimeWindow), ti.swarmValues)
			if rate > float64(s.TimeWindow) {
				t.Errorf("Failed to drop below maxhits calcTotalRequestRate: rate=%v but should be less than %v", rate, s.MaxHits)
			}
		})

	}
}

func TestTwoSwarms(t *testing.T) {
	//log.SetLevel(log.DebugLevel)

	l := sync.Mutex{}
	leaveTimeout := 1 * time.Second
	leaveFunction := func() {
		time.Sleep(leaveTimeout)
		l.Unlock()
	}

	t.Run("two swarms, one backend", func(t *testing.T) {
		l.Lock()
		defer leaveFunction()

		s := Settings{
			Type:       ClusterRatelimit,
			MaxHits:    4,
			TimeWindow: 1 * time.Second,
		}

		sw1, err := newFakeSwarm("n1", leaveTimeout)
		if err != nil {
			t.Fatalf("Failed to start swarm1: %v", err)
		}
		sw2, err := newFakeSwarm("n2", leaveTimeout)
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
		backend1 := "TestTwoSwarms backend1"
		backend2 := "TestTwoSwarms backend2"

		for i := 0; i <= s.MaxHits; i++ {
			if i%2 == 0 && !crl1sw1.Allow(backend1) {
				t.Errorf("1.1 %s not allowed but should", backend1)
			}

			if i%2 != 0 && !crl1sw2.Allow(backend1) {
				t.Errorf("2.1 %s not allowed but should", backend1)
			}
		}

		time.Sleep(100 * time.Millisecond)
		crl1sw2.Allow(backend1)
		crl1sw1.Allow(backend1)

		time.Sleep(100 * time.Millisecond)
		if crl1sw2.Allow(backend1) {
			t.Errorf("2.2 %s allowed but should not", backend1)
		}

		time.Sleep(100 * time.Millisecond)
		if crl1sw1.Allow(backend1) {
			t.Errorf("1.2 %s allowed but should not", backend1)
		}

		if !crl1sw2.Allow(backend2) {
			t.Errorf("%s should not interfere with %s", backend1, backend2)
		}
	})

	//func TestTwoSwarms2(t *testing.T) {
	//log.SetLevel(log.DebugLevel)
	t.Run("two swarms, two backends", func(t *testing.T) {
		l.Lock()
		defer leaveFunction()

		s := Settings{
			Type:       ClusterRatelimit,
			MaxHits:    4,
			TimeWindow: 1 * time.Second,
		}

		sw1, err := newFakeSwarm("n1", leaveTimeout)
		if err != nil {
			t.Fatalf("Failed to start swarm1: %v", err)
		}
		sw2, err := newFakeSwarm("n2", leaveTimeout)
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
		backend1 := "TestTwoSwarms2 backend1"
		backend2 := "TestTwoSwarms2 backend2"

		for i := 0; i <= s.MaxHits; i++ {
			if i%2 == 0 {
				if !crl1sw1.Allow(backend1) {
					t.Errorf("1.1 %d %s not allowed but should", i, backend1)
				}
				if !crl1sw1.Allow(backend2) {
					t.Errorf("1.2 %d %s not allowed but should", i, backend2)
				}
			} else {
				if !crl1sw2.Allow(backend1) {
					t.Errorf("2.1 %d %s not allowed but should", i, backend1)
				}
				if !crl1sw2.Allow(backend2) {
					t.Errorf("2.2 %d %s not allowed but should", i, backend2)
				}
			}
		}

		time.Sleep(100 * time.Millisecond)
		crl1sw2.Allow(backend1)
		crl1sw1.Allow(backend1)
		time.Sleep(100 * time.Millisecond)
		crl1sw1.Allow(backend2)
		crl1sw2.Allow(backend2)

		if crl1sw1.Allow(backend1) {
			t.Errorf("1.2 1 %s not allowed but should", backend1)
		}
		if crl1sw1.Allow(backend2) {
			t.Errorf("1.2 2 %s allowed but should not", backend2)
		}
		if crl1sw2.Allow(backend1) {
			t.Errorf("2.1 1 %s allowed but should not", backend1)
		}
		if crl1sw2.Allow(backend2) {
			t.Errorf("2.1 2 %s allowed but should not", backend2)
		}
	})

	//func TestTwoSwarmsMaze(t *testing.T) {
	//log.SetLevel(log.DebugLevel)
	t.Run("two swarms, one backend, maze", func(t *testing.T) {
		l.Lock()
		defer leaveFunction()

		s := Settings{
			Type:       ClusterRatelimit,
			MaxHits:    100,
			TimeWindow: 1 * time.Second,
		}

		sw1, err := newFakeSwarm("n1", leaveTimeout)
		if err != nil {
			t.Fatalf("Failed to start swarm1: %v", err)
		}
		sw2, err := newFakeSwarm("n2", leaveTimeout)
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
		backend1 := "TestTwoSwarmsMaze backend1"

		//t.Run("two swarm peers, single ratelimit backend", func(t *testing.T) {
		for i := 0; i <= s.MaxHits; i++ {
			if i%2 == 0 && !crl1sw1.Allow(backend1) {
				t.Errorf("1.%d %s not allowed but should", i, backend1)
			}

			if i%2 != 0 && !crl1sw2.Allow(backend1) {
				t.Errorf("2.%d %s not allowed but should", i, backend1)
			}
		}
		// update swarm once again to be predictable
		time.Sleep(150 * time.Millisecond)
		crl1sw1.Allow(backend1)
		crl1sw2.Allow(backend1)
		time.Sleep(150 * time.Millisecond)
		crl1sw1.Allow(backend1)
		crl1sw2.Allow(backend1)
		time.Sleep(150 * time.Millisecond)

		if crl1sw1.Allow(backend1) {
			t.Errorf("1 %s allowed but should not", backend1)
		}
		if crl1sw2.Allow(backend1) {
			t.Errorf("2 %s allowed but should not", backend1)
		}

	})
}
