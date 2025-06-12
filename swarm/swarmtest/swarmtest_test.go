package swarmtest_test

import (
	"net"
	"testing"
	"time"

	"github.com/zalando/skipper/swarm/swarmtest"
)

func TestSwarmNode(t *testing.T) {
	nodeName := "node1"
	addrPort, err := net.ResolveUDPAddr("udp", "127.0.0.1:9500")
	if err != nil {
		t.Fatalf("Failed to ResolveUDPAddr: %v", err)
	}
	ipStr := addrPort.IP.String()
	port := addrPort.Port

	node, err := swarmtest.NewTestNode(nodeName, ipStr, port)
	if err != nil {
		t.Fatalf("Failed to create test node: %v", err)
	}

	if healthScore := node.GetHealthScore(); healthScore != 0 {
		t.Fatalf("Failed to be healthy, healthscore not 0, got: %v", healthScore)
	}

	if numMembers := node.NumMembers(); numMembers != 1 {
		t.Fatalf("Failed to get expected number (1) of nodes, got: %v", numMembers)
	}

	if d, err := node.Ping(nodeName, addrPort); err != nil {
		t.Fatalf("Failed to ping node '%s': %v", nodeName, err)
	} else {
		t.Logf("Ping to '%s' took %s", nodeName, d)
	}

	if err := node.Leave(100 * time.Millisecond); err != nil {
		t.Fatalf("Failed to leave: %v", err)
	}

	if err := node.Shutdown(); err != nil {
		t.Fatalf("Failed to shotdown: %v", err)
	}
}
