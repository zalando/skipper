package valkeytest

import (
	"context"
	"sync"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
)

// valkeyTestNetwork is a network for valkey containers.
// Containers on this network will use the same port and different addresses
// unlike containers on the host network, which use different ports and the same loopback address.
// The network is created on the first use and removed when the last container is removed.
var valkeyTestNetwork = testNetwork{}

type testNetwork struct {
	mu         sync.Mutex
	network    *testcontainers.DockerNetwork
	err        error
	containers int
}

func (tn *testNetwork) acquire() (*testcontainers.DockerNetwork, error) {
	tn.mu.Lock()
	defer tn.mu.Unlock()

	if tn.err != nil {
		return nil, tn.err
	} else if tn.network != nil {
		tn.containers++
		return tn.network, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tn.network, tn.err = network.New(ctx)
	if tn.network != nil {
		tn.containers = 1
	}
	return tn.network, tn.err
}

func (tn *testNetwork) release() {
	tn.mu.Lock()
	defer tn.mu.Unlock()

	if tn.network != nil {
		if tn.containers > 1 {
			tn.containers--
		} else if tn.containers == 1 {
			tn.err = tn.network.Remove(context.Background())
			tn.network = nil
			tn.containers = 0
		} else {
			panic("release called without acquire")
		}
	}
}
