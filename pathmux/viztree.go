package pathmux

// Exploded version of the pathmux tree designed for being easy to use in a visualization.
// Simple wildcard nodes are represented by the ':' prefix and free wildcard nodes with the '*' prefix.
type VizTree struct {
	Path     string     // string representation of the node path
	Children []*VizTree // children nodes of the current node
	CanMatch bool       // flag that is set to true if the node has a matcher

	// The wildcard names still waiting to be processed, should be always nil after the end of the conversion.
	leafWildcardNames []string
}

// Creates a new visualization tree from a pathmux.Tree.
func NewVizTree(tree *Tree) *VizTree {
	vizTree := aggregateTree((*node)(tree), "/")
	return vizTree[0]
}

func aggregateTree(tree *node, previousPath string) []*VizTree {
	if tree == nil {
		return nil
	}
	var currentNode, middleNode *VizTree
	nextPath := previousPath + tree.path
	// On every slash we create new nodes if the previous Path has a value, otherwise we ignore it
	if tree.path == "/" {
		if previousPath != "" {
			currentNode = &VizTree{
				Path:              previousPath,
				CanMatch:          tree.leafValue != nil,
				leafWildcardNames: tree.leafWildcardNames,
			}
		}
		nextPath = ""
	} else {
		// This is to handle intermediate nodes that are matched without a slash
		if tree.leafValue != nil {
			middleNode = &VizTree{
				Path:              nextPath,
				CanMatch:          true,
				leafWildcardNames: tree.leafWildcardNames,
			}
		}
	}
	numberOfChildren := len(tree.staticChild)
	children := make([]*VizTree, 0, numberOfChildren)
	for i := 0; i < numberOfChildren; i++ {
		childrenNodes := aggregateTree(tree.staticChild[i], nextPath)
		children = append(children, childrenNodes...)
	}
	childrenNodes := processWildCardNode(tree.wildcardChild)
	children = append(children, childrenNodes...)

	if tree.catchAllChild != nil && tree.catchAllChild.leafValue != nil {
		children = append(children,
			&VizTree{
				Path:              "*" + tree.catchAllChild.path,
				CanMatch:          true,
				leafWildcardNames: sliceLeafNames(tree.catchAllChild.leafWildcardNames),
			})
	}
	if currentNode != nil {
		// Keep lifting the leaf wildcard names for later processing
		leafWildcardNames := liftLeafNames(childrenNodes)
		if len(leafWildcardNames) != 0 {
			currentNode.leafWildcardNames = leafWildcardNames
		}
		currentNode.Children = children
		return []*VizTree{currentNode}
	}
	if middleNode != nil {
		// Keep lifting the leaf wildcard names for later processing
		leafWildcardNames := liftLeafNames(childrenNodes)
		// Prevent duplication of intermediate nodes
		if previousNode := findNode(children, middleNode.Path); previousNode == nil {
			if len(leafWildcardNames) != 0 {
				middleNode.leafWildcardNames = leafWildcardNames
			}
			children = append(children, middleNode)
		} else {
			previousNode.CanMatch = true
			if len(leafWildcardNames) != 0 {
				previousNode.leafWildcardNames = leafWildcardNames
			}
		}
	}
	return children
}

func processWildCardNode(tree *node) []*VizTree {
	if tree == nil {
		return nil
	}
	numberOfChildren := len(tree.staticChild)
	if len(tree.leafWildcardNames) != 0 && numberOfChildren == 0 {
		currentPath := tree.leafWildcardNames[len(tree.leafWildcardNames)-1]
		leafWildcardNames := sliceLeafNames(tree.leafWildcardNames)
		return []*VizTree{{
			Path:              ":" + currentPath,
			CanMatch:          tree.leafValue != nil,
			leafWildcardNames: leafWildcardNames,
		}}
	}
	children := make([]*VizTree, 0, numberOfChildren)
	for i := 0; i < numberOfChildren; i++ {
		childrenNodes := aggregateTree(tree.staticChild[i], "")
		for j := 0; j < len(childrenNodes); j++ {
			child := childrenNodes[j]
			if len(child.leafWildcardNames) != 0 {
				currentPath := ":" + child.leafWildcardNames[len(child.leafWildcardNames)-1]
				leafWildcardNames := sliceLeafNames(child.leafWildcardNames)
				child.leafWildcardNames = nil
				if previousNode := findNode(children, currentPath); previousNode == nil {
					canMatch := tree.leafValue != nil
					if len(tree.leafWildcardNames) > 0 {
						// If these do not match the leafValue is not for this node
						checkMatcherPath := ":" + tree.leafWildcardNames[len(tree.leafWildcardNames)-1]
						canMatch = checkMatcherPath == currentPath
					}
					child = &VizTree{
						Path:              currentPath,
						Children:          []*VizTree{child},
						CanMatch:          canMatch,
						leafWildcardNames: leafWildcardNames,
					}
				} else {
					previousNode.Children = append(previousNode.Children, child)
					child = nil
				}
			}
			if child != nil {
				children = append(children, child)
			}
		}
	}

	return children
}

func findNode(children []*VizTree, path string) *VizTree {
	for _, child := range children {
		if path == child.Path {
			return child
		}
	}
	return nil
}

func liftLeafNames(children []*VizTree) []string {
	var leafWildcardNames []string
	for _, child := range children {
		if len(child.leafWildcardNames) != 0 {
			leafWildcardNames = child.leafWildcardNames
			// Only done for sanity checks, later we can test if everything was properly lifted
			child.leafWildcardNames = nil
		}
	}
	return leafWildcardNames
}

func sliceLeafNames(names []string) []string {
	if len(names) < 2 {
		return nil
	}
	return names[:len(names)-1]
}
