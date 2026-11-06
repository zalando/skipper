package routing

import "net/http"

// An exact-host predicate must reject other hosts without side effects.
// Only the first custom predicate can be indexed: earlier predicates may
// change the host or have other effects even when the leaf does not match.
type hostKeyedPredicate interface {
	MatchHosts() []string
}

func (m *pathMatcher) indexHosts() {
	if len(m.leaves) < 4 {
		return
	}
	for i, l := range m.leaves {
		var hosts []string
		if len(l.predicates) > 0 {
			if p, ok := l.predicates[0].(hostKeyedPredicate); ok {
				hosts = p.MatchHosts()
			}
		}
		if len(hosts) == 0 {
			if m.hostLeaves != nil {
				m.otherLeaves = append(m.otherLeaves, i)
			}
			continue
		}
		if m.hostLeaves == nil {
			m.hostLeaves = make(map[string][]int)
			for j := 0; j < i; j++ {
				m.otherLeaves = append(m.otherLeaves, j)
			}
		}
		for _, host := range hosts {
			positions := m.hostLeaves[host]
			if len(positions) == 0 || positions[len(positions)-1] != i {
				m.hostLeaves[host] = append(positions, i)
			}
		}
	}
}

func (m *pathMatcher) matchHostLeaves(req *http.Request, path, exactPath string) *leafMatcher {
	host := req.Host
	positions := m.hostLeaves[host]
	for i, j := 0, 0; i < len(positions) || j < len(m.otherLeaves); {
		var pos int
		if j == len(m.otherLeaves) || i < len(positions) && positions[i] < m.otherLeaves[j] {
			pos = positions[i]
			i++
		} else {
			pos = m.otherLeaves[j]
			j++
		}
		l := m.leaves[pos]
		if matchLeaf(l, req, path, exactPath) {
			return l
		}
		if req.Host != host {
			return matchLeaves(m.leaves[pos+1:], req, path, exactPath)
		}
	}
	return nil
}
