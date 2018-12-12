package swarm

import (
	"fmt"
	"net"

	log "github.com/sirupsen/logrus"
)

func mapNodesToAddresses(n []*NodeInfo) []string {
	var s []string
	for i := range n {
		s = append(s, fmt.Sprintf("%v:%d", n[i].Addr, n[i].Port))
	}

	return s
}
func getSelf(nodes []*NodeInfo) *NodeInfo {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatalf("SWARM: Failed to get addr: %v", err)
	}

	for _, ni := range nodes {
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				log.Errorf("SWARM: could not parse cidr: %v", err)
				continue
			}
			if ip.Equal(ni.Addr) {
				return ni
			}
		}
	}
	return nil
}

func reverse(b [][]byte) [][]byte {
	for i := range b[:len(b)/2] {
		b[i], b[len(b)-1-i] = b[len(b)-1-i], b[i]
	}

	return b
}

func takeMaxLatest(b [][]byte, overhead, max int) [][]byte {
	var (
		bb   [][]byte
		size int
	)

	for i := range b {
		bli := b[len(b)-i-1]

		if size+len(bli)+overhead > max {
			break
		}

		bb = append(bb, bli)
		size += len(bli) + overhead
	}

	return reverse(bb)
}
