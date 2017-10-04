package pathmux

type vizNode struct {
	path     string
	children []*vizNode
	canMatch bool

	// The wildcard names still waiting to be processed, should be always nil after the end of the conversion.
	leafWildcardNames []string
}

type VizTree vizNode

func NewVizTree(tree *Tree) (*VizTree) {
	vizTree := aggregateTree((*node)(tree), "/")
	return (*VizTree)(vizTree[0])
}

func aggregateTree(tree *node, previousPath string) ([]*vizNode) {
	if tree == nil {
		return nil
	}
	var currentNode, middleNode *vizNode
	nextPath := previousPath + tree.path
	// On every slash we create new nodes if the previous path has a value, otherwise we ignore it
	if tree.path == "/" {
		if previousPath != "" {
			currentNode = &vizNode{previousPath, nil, tree.leafValue != nil, tree.leafWildcardNames}
		}
		nextPath = ""
	} else {
		// This is to handle intermediate nodes that are matched without a slash
		if tree.leafValue != nil {
			middleNode = &vizNode{nextPath, nil, true, tree.leafWildcardNames}
		}
	}
	numberOfChildren := len(tree.staticChild)
	children := make([]*vizNode, 0, numberOfChildren)
	for i := 0; i < numberOfChildren; i++ {
		childrenNodes := aggregateTree(tree.staticChild[i], nextPath)
		children = append(children, childrenNodes...)
	}
	childrenNodes := processWildCardNode(tree.wildcardChild)
	// Keep lifting the leaf wildcard names for later processing
	leafWildcardNames := getLeafNames(childrenNodes, currentNode != nil || middleNode != nil)
	children = append(children, childrenNodes...)

	if tree.catchAllChild != nil && tree.catchAllChild.leafValue != nil {
		children = append(children, &vizNode{"*" + tree.catchAllChild.path, nil, true, sliceLeafNames(tree.catchAllChild.leafWildcardNames)})
	}
	if currentNode != nil {
		if leafWildcardNames != nil {
			currentNode.leafWildcardNames = leafWildcardNames
		}
		currentNode.children = children
		return []*vizNode{currentNode}
	}
	if middleNode != nil {
		// Prevent duplication of intermediate nodes
		if previousNode := getNode(children, middleNode.path); previousNode == nil {
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

func (n *vizNode) child(path string) (*vizNode) {
	for i := 0; i < len(n.children); i++ {
		child := n.children[i]
		if path == child.path {
			return child
		}
	}
	return nil
}
func processWildCardNode(tree *node) ([]*vizNode) {
	if tree == nil {
		return nil
	}
	numberOfChildren := len(tree.staticChild)
	if tree.leafWildcardNames != nil && numberOfChildren == 0 {
		currentPath, leafWildcardNames := tree.leafWildcardNames[len(tree.leafWildcardNames)-1], sliceLeafNames(tree.leafWildcardNames)
		return []*vizNode{{":" + currentPath, nil, tree.leafValue != nil, leafWildcardNames}}
	}
	children := make([]*vizNode, 0, numberOfChildren)
	for i := 0; i < numberOfChildren; i++ {
		childrenNodes := aggregateTree(tree.staticChild[i], "")
		for j := 0; j < len(childrenNodes); j++ {
			child := childrenNodes[j]
			if child.leafWildcardNames != nil {
				currentPath, leafWildcardNames := ":"+child.leafWildcardNames[len(child.leafWildcardNames)-1], sliceLeafNames(child.leafWildcardNames)
				child.leafWildcardNames = nil
				if previousNode := getNode(children, currentPath); previousNode == nil {
					child = &vizNode{currentPath, []*vizNode{child}, tree.leafValue != nil, leafWildcardNames}
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
func getNode(children []*vizNode, path string) (*vizNode) {
	for i := 0; i < len(children); i++ {
		child := children[i]
		if path == child.path {
			return child
		}
	}
	return nil
}
func getLeafNames(children []*vizNode, removeFromChildren bool) ([]string) {
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
func sliceLeafNames(names []string) ([]string) {
	if names == nil || len(names) < 2 {
		return nil
	}
	return names[:len(names)-1]
}
