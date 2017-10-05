package pathmux

// Exploded version of the pathmux tree designed for being easy to use in a visualization.
// Simple wildcard nodes are represented by the ':' prefix and free wildcard nodes with the '*' prefix.
type VizTree struct {
	Path     string // string representation of the node path
	children []*VizTree
	canMatch bool

	// The wildcard names still waiting to be processed, should be always nil after the end of the conversion.
	leafWildcardNames []string
}

// Creates a new visualization tree from a pathmux.Tree.
func NewVizTree(tree *Tree) *VizTree {
	vizTree := aggregateTree((*node)(tree), "/")
	return vizTree[0]
}

// Return a child referenced by direct Path
//
// Example:
//
// - in a tree such as: / -> images -> abc
//
// looking for images in the root tree would yield the images subtree.
func (n *VizTree) Child(path string) *VizTree {
	return findNode(n.children, path)
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
			currentNode = &VizTree{previousPath, nil, tree.leafValue != nil, tree.leafWildcardNames}
		}
		nextPath = ""
	} else {
		// This is to handle intermediate nodes that are matched without a slash
		if tree.leafValue != nil {
			middleNode = &VizTree{nextPath, nil, true, tree.leafWildcardNames}
		}
	}
	numberOfChildren := len(tree.staticChild)
	children := make([]*VizTree, 0, numberOfChildren)
	for i := 0; i < numberOfChildren; i++ {
		childrenNodes := aggregateTree(tree.staticChild[i], nextPath)
		children = append(children, childrenNodes...)
	}
	childrenNodes := processWildCardNode(tree.wildcardChild)
	// Keep lifting the leaf wildcard names for later processing
	leafWildcardNames := getLeafNames(childrenNodes, currentNode != nil || middleNode != nil)
	children = append(children, childrenNodes...)

	if tree.catchAllChild != nil && tree.catchAllChild.leafValue != nil {
		children = append(children, &VizTree{"*" + tree.catchAllChild.path, nil, true, sliceLeafNames(tree.catchAllChild.leafWildcardNames)})
	}
	if currentNode != nil {
		if leafWildcardNames != nil {
			currentNode.leafWildcardNames = leafWildcardNames
		}
		currentNode.children = children
		return []*VizTree{currentNode}
	}
	if middleNode != nil {
		// Prevent duplication of intermediate nodes
		if previousNode := findNode(children, middleNode.Path); previousNode == nil {
			if leafWildcardNames != nil {
				middleNode.leafWildcardNames = leafWildcardNames
			}
			children = append(children, middleNode)
		} else {
			previousNode.canMatch = true
			if leafWildcardNames != nil {
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
	if tree.leafWildcardNames != nil && numberOfChildren == 0 {
		currentPath, leafWildcardNames := tree.leafWildcardNames[len(tree.leafWildcardNames)-1], sliceLeafNames(tree.leafWildcardNames)
		return []*VizTree{{":" + currentPath, nil, tree.leafValue != nil, leafWildcardNames}}
	}
	children := make([]*VizTree, 0, numberOfChildren)
	for i := 0; i < numberOfChildren; i++ {
		childrenNodes := aggregateTree(tree.staticChild[i], "")
		for j := 0; j < len(childrenNodes); j++ {
			child := childrenNodes[j]
			if child.leafWildcardNames != nil {
				currentPath, leafWildcardNames := ":"+child.leafWildcardNames[len(child.leafWildcardNames)-1], sliceLeafNames(child.leafWildcardNames)
				child.leafWildcardNames = nil
				if previousNode := findNode(children, currentPath); previousNode == nil {
					child = &VizTree{currentPath, []*VizTree{child}, tree.leafValue != nil, leafWildcardNames}
				} else {
					previousNode.children = append(previousNode.children, child)
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
	for i := 0; i < len(children); i++ {
		child := children[i]
		if path == child.Path {
			return child
		}
	}
	return nil
}

func getLeafNames(children []*VizTree, removeFromChildren bool) []string {
	var leafWildcardNames []string
	for i := 0; i < len(children); i++ {
		if children[i].leafWildcardNames != nil {
			leafWildcardNames = children[i].leafWildcardNames
			if removeFromChildren {
				children[i].leafWildcardNames = nil
			}
		}
	}
	return leafWildcardNames
}

func sliceLeafNames(names []string) []string {
	if names == nil || len(names) < 2 {
		return nil
	}
	return names[:len(names)-1]
}
