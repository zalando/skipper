package swarm

import (
	"net"
	"reflect"
	"testing"
)

func TestNodeInfo_String(t *testing.T) {
	for _, tt := range []struct {
		msg      string
		name     string
		addr     net.IP
		port     int
		expected string
	}{{
		msg:      "all values set",
		name:     "host1",
		addr:     net.IPv4(127, 0, 0, 1),
		port:     8080,
		expected: "NodeInfo{host1, 127.0.0.1, 8080}",
	}, {
		msg:      "no port",
		name:     "host1",
		addr:     net.IPv4(127, 0, 0, 1),
		expected: "NodeInfo{host1, 127.0.0.1, 0}",
	}} {

		t.Run(tt.msg, func(t *testing.T) {
			ni := NodeInfo{
				Name: tt.name,
				Addr: tt.addr,
				Port: tt.port,
			}
			if got := ni.String(); got != tt.expected {
				t.Errorf("NodeInfo.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func Test_knownEntryPoint_Node(t *testing.T) {
	for _, tt := range []struct {
		msg      string
		self     *NodeInfo
		nodes    []*NodeInfo
		expected *NodeInfo
	}{{
		msg:      "all empty",
		self:     &NodeInfo{},
		nodes:    []*NodeInfo{},
		expected: &NodeInfo{},
	}, {
		msg: "only self",
		self: &NodeInfo{
			Name: "me",
			Addr: net.IPv4(172, 32, 2, 4),
			Port: 9292,
		},
		nodes: []*NodeInfo{},
		expected: &NodeInfo{
			Name: "me",
			Addr: net.IPv4(172, 32, 2, 4),
			Port: 9292,
		},
	}, {
		msg: "self and 1 friend",
		self: &NodeInfo{
			Name: "me",
			Addr: net.IPv4(172, 32, 2, 4),
			Port: 9292,
		},
		nodes: []*NodeInfo{
			&NodeInfo{
				Name: "friend",
				Addr: net.IPv4(172, 32, 2, 5),
				Port: 9292,
			},
		},
		expected: &NodeInfo{
			Name: "me",
			Addr: net.IPv4(172, 32, 2, 4),
			Port: 9292,
		},
	}, {
		msg: "self and 3 friends",
		self: &NodeInfo{
			Name: "me",
			Addr: net.IPv4(172, 32, 2, 4),
			Port: 9292,
		},
		nodes: []*NodeInfo{
			&NodeInfo{
				Name: "friend1",
				Addr: net.IPv4(172, 32, 2, 5),
				Port: 9292,
			}, &NodeInfo{
				Name: "friend2",
				Addr: net.IPv4(172, 32, 2, 6),
				Port: 9292,
			}, &NodeInfo{
				Name: "friend3",
				Addr: net.IPv4(172, 32, 2, 7),
				Port: 9292,
			},
		},
		expected: &NodeInfo{
			Name: "me",
			Addr: net.IPv4(172, 32, 2, 4),
			Port: 9292,
		},
	}} {
		t.Run(tt.msg, func(t *testing.T) {
			e := &knownEntryPoint{
				self:  tt.self,
				nodes: tt.nodes,
			}
			if got := e.Node(); !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("knownEntryPoint.Node() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func Test_knownEntryPoint_Nodes(t *testing.T) {
	for _, tt := range []struct {
		msg      string
		self     *NodeInfo
		nodes    []*NodeInfo
		expected []*NodeInfo
	}{{
		msg:      "all empty",
		self:     &NodeInfo{},
		nodes:    []*NodeInfo{},
		expected: []*NodeInfo{},
	}, {
		msg: "only self",
		self: &NodeInfo{
			Name: "me",
			Addr: net.IPv4(172, 32, 2, 4),
			Port: 9292,
		},
		nodes:    []*NodeInfo{},
		expected: []*NodeInfo{},
	}, {
		msg: "self and 1 friend",
		self: &NodeInfo{
			Name: "me",
			Addr: net.IPv4(172, 32, 2, 4),
			Port: 9292,
		},
		nodes: []*NodeInfo{
			&NodeInfo{
				Name: "friend",
				Addr: net.IPv4(172, 32, 2, 5),
				Port: 9292,
			},
		},
		expected: []*NodeInfo{
			&NodeInfo{
				Name: "friend",
				Addr: net.IPv4(172, 32, 2, 5),
				Port: 9292,
			},
		},
	}, {
		msg: "self and 3 friends",
		self: &NodeInfo{
			Name: "me",
			Addr: net.IPv4(172, 32, 2, 4),
			Port: 9292,
		},
		nodes: []*NodeInfo{
			&NodeInfo{
				Name: "friend1",
				Addr: net.IPv4(172, 32, 2, 5),
				Port: 9292,
			}, &NodeInfo{
				Name: "friend2",
				Addr: net.IPv4(172, 32, 2, 6),
				Port: 9292,
			}, &NodeInfo{
				Name: "friend3",
				Addr: net.IPv4(172, 32, 2, 7),
				Port: 9292,
			},
		},
		expected: []*NodeInfo{
			&NodeInfo{
				Name: "friend1",
				Addr: net.IPv4(172, 32, 2, 5),
				Port: 9292,
			}, &NodeInfo{
				Name: "friend2",
				Addr: net.IPv4(172, 32, 2, 6),
				Port: 9292,
			}, &NodeInfo{
				Name: "friend3",
				Addr: net.IPv4(172, 32, 2, 7),
				Port: 9292,
			},
		},
	}} {
		t.Run(tt.msg, func(t *testing.T) {
			e := &knownEntryPoint{
				self:  tt.self,
				nodes: tt.nodes,
			}
			if got := e.Nodes(); !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("knownEntryPoint.Nodes() = %v, expected %v", got, tt.expected)
			}
		})
	}
}
