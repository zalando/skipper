package routing

import (
	"net/http"
	"strings"
)

type hostIndex struct {
	hosts map[string][]int
	other []int
}

func literalHosts(rx string) []string {
	rx, ok := strings.CutPrefix(rx, "^(")
	if !ok {
		return nil
	}
	rx, ok = strings.CutSuffix(rx, ")$")
	if !ok {
		return nil
	}
	hosts := strings.Split(rx, "|")
	for i, h := range hosts {
		h, ok = strings.CutSuffix(h, "[.]?(:[0-9]+)?")
		if !ok || h == "" {
			return nil
		}
		for j := 0; j < len(h); j++ {
			if strings.HasPrefix(h[j:], "[.]") {
				j += 2
				continue
			}
			c := h[j]
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return nil
			}
		}
		hosts[i] = strings.ReplaceAll(h, "[.]", ".")
	}
	return hosts
}

func (m *pathMatcher) indexHosts() {
	if len(m.leaves) < 4 {
		return
	}
	for i, l := range m.leaves {
		var hosts []string
		if len(l.hostRxs) == 1 {
			hosts = literalHosts(l.hostRxs[0].String())
		}
		if len(hosts) == 0 {
			if m.hostIndex != nil {
				m.hostIndex.other = append(m.hostIndex.other, i)
			}
			continue
		}
		if m.hostIndex == nil {
			m.hostIndex = &hostIndex{hosts: make(map[string][]int)}
			for j := 0; j < i; j++ {
				m.hostIndex.other = append(m.hostIndex.other, j)
			}
		}
		// A literal ending in a dot can match with or without its optional extra dot.
		for _, h := range hosts {
			for _, key := range []string{h, strings.TrimSuffix(h, ".")} {
				positions := m.hostIndex.hosts[key]
				if len(positions) == 0 || positions[len(positions)-1] != i {
					m.hostIndex.hosts[key] = append(positions, i)
				}
			}
		}
	}
}

func hostKey(host string) string {
	if colon := strings.LastIndexByte(host, ':'); colon >= 0 && colon < len(host)-1 {
		port := host[colon+1:]
		if strings.IndexFunc(port, func(c rune) bool { return c < '0' || c > '9' }) == -1 {
			host = host[:colon]
		}
	}
	return strings.TrimSuffix(host, ".")
}

func (m *pathMatcher) matchHostLeaves(req *http.Request, path, exactPath string) *leafMatcher {
	host := req.Host
	positions := m.hostIndex.hosts[hostKey(host)]
	other := m.hostIndex.other
	for i, j := 0, 0; i < len(positions) || j < len(other); {
		var pos int
		if j == len(other) || i < len(positions) && positions[i] < other[j] {
			pos = positions[i]
			i++
		} else {
			pos = other[j]
			j++
		}
		l := m.leaves[pos]
		if matchLeaf(l, req, path, exactPath) {
			return l
		}
		// A rejecting custom predicate can change the host for subsequent leaves.
		if req.Host != host {
			return matchLeaves(m.leaves[pos+1:], req, path, exactPath)
		}
	}
	return nil
}
