package routing

import (
	"net/http"
	"strings"
)

// Only recognize the literal alternatives emitted by the Kubernetes dataclient.
// All other expressions still use the full scan.
func literalHosts(expr string) []string {
	expr, ok := strings.CutPrefix(expr, "^(")
	if !ok {
		return nil
	}
	expr, ok = strings.CutSuffix(expr, ")$")
	if !ok {
		return nil
	}

	var hosts []string
	for part := range strings.SplitSeq(expr, "|") {
		lit, ok := strings.CutSuffix(part, "[.]?(:[0-9]+)?")
		if !ok || lit == "" {
			return nil
		}
		host := strings.ReplaceAll(lit, "[.]", ".")
		for i := range len(host) {
			c := host[i]
			if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '-' || c == '.') {
				return nil
			}
		}
		if strings.ReplaceAll(host, ".", "[.]") != lit {
			return nil
		}
		hosts = append(hosts, host)
	}
	return hosts
}

func hostKey(host string) string {
	if colon := strings.LastIndexByte(host, ':'); colon >= 0 && colon < len(host)-1 {
		port := host[colon+1:]
		if strings.IndexFunc(port, func(c rune) bool { return c < '0' || c > '9' }) < 0 {
			host = host[:colon]
		}
	}
	return strings.TrimSuffix(host, ".")
}

func (m *pathMatcher) indexHosts() {
	for i, l := range m.leaves {
		var hosts []string
		if len(l.hostRxs) == 1 {
			hosts = literalHosts(l.hostRxs[0].String())
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
			// A literal ending in a dot has two different normalized keys,
			// depending on whether the optional trailing dot is present.
			for _, key := range []string{strings.TrimSuffix(host, "."), host} {
				positions := m.hostLeaves[key]
				if len(positions) == 0 || positions[len(positions)-1] != i {
					m.hostLeaves[key] = append(positions, i)
				}
			}
		}
	}
}

func (m *pathMatcher) match(req *http.Request, path, exactPath string) *leafMatcher {
	if m.hostLeaves == nil {
		return matchLeaves(m.leaves, req, path, exactPath)
	}

	host := req.Host
	positions := m.hostLeaves[hostKey(host)]
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
