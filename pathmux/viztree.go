package pathmux

type vizNode struct {
	path     string
	children []*vizNode
	canMatch bool
}

func MakeVizTree(tree *node) (*vizNode) {
	return aggregateTree(tree, nil, "/")
}

func aggregateTree(tree *node, vizTree *vizNode, previousPath string) (*vizNode) {
	numberOfChildren := len(tree.staticChild)
	if numberOfChildren == 0 {
		// No leafValue means this node should be ignored
		if tree.leafValue == nil {
			var previousNode *vizNode
			if vizTree != nil {
				previousNode = vizTree.child(previousPath)
			}
			if tree.catchAllChild != nil && tree.catchAllChild.leafValue != nil {
				catchAllChild := &vizNode{"*" + tree.catchAllChild.path, nil, true}
				if previousNode != nil {
					previousNode.children = append(previousNode.children, catchAllChild)
				} else if vizTree != nil {
					vizTree.children = append(vizTree.children, catchAllChild)
				} else {
					return catchAllChild
				}
			}
			return vizTree
		}
		var childPath string
		if tree.path == "/" {
			childPath = previousPath
		} else {
			childPath = previousPath + tree.path
		}
		newChildNode := &vizNode{childPath, nil, true}
		if vizTree != nil {
			vizTree.children = append(vizTree.children, newChildNode)
		} else {
			vizTree = newChildNode
		}
		return vizTree
	}
	var newChildNode *vizNode
	nextPath := previousPath + tree.path
	if tree.path == "/" {
		nextPath = ""
		var previousNode *vizNode
		if vizTree != nil {
			previousNode = vizTree.child(previousPath)
		}
		if previousNode == nil {
			newChildNode = &vizNode{previousPath, nil, tree.leafValue != nil}
			if vizTree != nil {
				vizTree.children = append(vizTree.children, newChildNode)
			} else {
				vizTree = newChildNode
			}
		} else {
			newChildNode = previousNode
		}
	} else {
		if tree.leafValue != nil {
			newChildNode = &vizNode{nextPath, nil, true}
			if vizTree != nil {
				vizTree.children = append(vizTree.children, newChildNode)
			} else {
				vizTree = newChildNode
			}
		}
		newChildNode = vizTree

	}
	for i := 0; i < numberOfChildren; i++ {
		child := tree.staticChild[i]
		aggregateTree(child, newChildNode, nextPath)
	}
	if tree.catchAllChild != nil && tree.catchAllChild.leafValue != nil {
		catchAllChild := &vizNode{"*" + tree.catchAllChild.path, nil, true}
		newChildNode.children = append(newChildNode.children, catchAllChild)
	}
	return vizTree
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
