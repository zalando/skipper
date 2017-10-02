package pathmux

type vizNode struct {
	path     string
	children []*vizNode
	canMatch bool
}

func MakeVizTree(tree *node) (*vizNode) {
	vizTree, _ := aggregateTree(tree, "/")
	return vizTree
}

func aggregateTree(tree *node, previousPath string) (*vizNode, []*vizNode) {
	if tree == nil {
		return nil, nil
	}
	numberOfChildren := len(tree.staticChild)
	var currentNode, middleNode *vizNode
	middleNode = nil
	nextPath := previousPath + tree.path
	if tree.path == "/" {
		nextPath = ""
		currentNode = &vizNode{previousPath, nil, tree.leafValue != nil}
	} else {
		if tree.leafValue != nil {
			middleNode = &vizNode{nextPath, nil, true}
		}
	}
	children := make([]*vizNode, 0, numberOfChildren)
	for i := 0; i < numberOfChildren; i++ {
		childNode, grandsonNodes := aggregateTree(tree.staticChild[i], nextPath)
		if  grandsonNodes != nil {
			children = append(children, grandsonNodes...)
		}
		if  childNode != nil {
			children = append(children, childNode)
		}
	}

	if tree.catchAllChild != nil && tree.catchAllChild.leafValue != nil {
		catchAllChild := &vizNode{"*" + tree.catchAllChild.path, nil, true}
		children = append(children, catchAllChild)
	}
	if currentNode != nil {
		currentNode.children = children
		return currentNode, nil
	}
	if middleNode != nil {
		if clearMiddleNodes(children, middleNode.path) {
			return nil, children
		}
	}
	return middleNode, children
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
func clearMiddleNodes(children []*vizNode, path string) (bool) {
	for i := 0; i < len(children); i++ {
		child := children[i]
		if path == child.path {
			child.canMatch = true
			return true
		}
	}
	return false
}
