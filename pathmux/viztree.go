package pathmux

// VizTree exploded version of the pathmux tree designed for being easy to use in a visualization.
// Simple wildcard nodes are represented by the ':' prefix and free wildcard nodes with the '*' prefix.
type VizTree struct {
	Path     string     // string representation of the node path
	Children []*VizTree // children nodes of the current node
	CanMatch bool       // flag that is set to true if the node has a matcher
}

// NewVizTree creates a new visualization tree from a pathmux.Tree.
func NewVizTree(tree *Tree) *VizTree {
	panic("not implemented anymore")
}
