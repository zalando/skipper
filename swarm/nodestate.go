package swarm

// NodeState represents the current state of a cluster node known by
// this instance.
type NodeState int

const (
	Initial NodeState = iota
	Connected
	Disconnected
)
