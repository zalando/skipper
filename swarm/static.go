package swarm

type StaticSwarm struct {
	self *NodeInfo
	all  []*NodeInfo
}

func NewStaticSwarm(self *NodeInfo, all []*NodeInfo) *StaticSwarm {
	return &StaticSwarm{
		self: self,
		all:  all,
	}
}
func (s *StaticSwarm) Self() *NodeInfo {
	return s.self
}
func (s *StaticSwarm) GetNodeInfo() ([]*NodeInfo, error) {
	return s.all, nil
}
