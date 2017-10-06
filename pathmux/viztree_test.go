package pathmux

import (
	"reflect"
	"sort"
	"testing"
)

func addPathToTree(t *testing.T, tree *Tree, path string) {
	t.Logf("Adding path %s", path)
	err := tree.Add(path, true)
	if err != nil {
		t.Error(err)
	}
}

func TestVizTree(t *testing.T) {
	tree := &Tree{path: "/"}

	addPathToTree(t, tree, "/")
	addPathToTree(t, tree, "/i")
	addPathToTree(t, tree, "/i/:aaa")
	addPathToTree(t, tree, "/images/abc.jpg")
	addPathToTree(t, tree, "/images/abc.jpg/:size")
	addPathToTree(t, tree, "/images/:imgname")
	addPathToTree(t, tree, "/images")
	addPathToTree(t, tree, "/images/*path")
	addPathToTree(t, tree, "/ima")
	addPathToTree(t, tree, "/ima/:par")
	addPathToTree(t, tree, "/images1")
	addPathToTree(t, tree, "/images1/*path1")
	addPathToTree(t, tree, "/images2")
	addPathToTree(t, tree, "/apples")
	addPathToTree(t, tree, "/app/les")
	addPathToTree(t, tree, "/apples1")
	addPathToTree(t, tree, "/appeasement")
	addPathToTree(t, tree, "/appealing")
	addPathToTree(t, tree, "/date/:year/:month")
	addPathToTree(t, tree, "/date/:year/month")
	addPathToTree(t, tree, "/date/:year/:month/abc")
	addPathToTree(t, tree, "/date/:year/:month/:post")
	addPathToTree(t, tree, "/date/:year/:month/*post")
	addPathToTree(t, tree, "/:page")
	addPathToTree(t, tree, "/:page/:index")
	addPathToTree(t, tree, "/post/:post/page/:page")
	addPathToTree(t, tree, "/plaster")
	addPathToTree(t, tree, "/users/:pk/:related")
	addPathToTree(t, tree, "/users/:id/updatePassword")
	addPathToTree(t, tree, "/:something/abc")
	addPathToTree(t, tree, "/:something/def")

	vizTree := NewVizTree(tree)
	if !vizTree.CanMatch {
		t.Fatalf("/ should match")
	}
	testChildren(t, vizTree, "i", "ima", "images", "images1", "images2", "apples", "apples1", "appeasement", "appealing", "app", "date", "plaster", ":page", "post", "users", ":something")
	testIfChildMatches(t, vizTree, "i")
	testChildren(t, vizTree.child("i"), ":aaa")
	testIfChildMatches(t, vizTree.child("i"), ":aaa")
	testIfChildMatches(t, vizTree, "images")
	testChildren(t, vizTree.child("images"), "abc.jpg", "*path", ":imgname")
	testIfChildMatches(t, vizTree.child("images"), "abc.jpg")
	testIfChildMatches(t, vizTree.child("images"), ":imgname")
	testIfChildMatches(t, vizTree.child("images"), "*path")
	testChildren(t, vizTree.child("images").child("abc.jpg"), ":size")
	testIfChildMatches(t, vizTree.child("images").child("abc.jpg"), ":size")
	testIfChildMatches(t, vizTree, "ima")
	testChildren(t, vizTree.child("ima"), ":par")
	testIfChildMatches(t, vizTree.child("ima"), ":par")
	testIfChildMatches(t, vizTree, "images1")
	testIfChildMatches(t, vizTree, "images2")
	testChildren(t, vizTree.child("images1"), "*path1")
	testIfChildMatches(t, vizTree.child("images1"), "*path1")
	testIfChildDoesNotMatches(t, vizTree, "app")
	testChildren(t, vizTree.child("app"), "les")
	testIfChildMatches(t, vizTree.child("app"), "les")
	testIfChildMatches(t, vizTree, "apples")
	testIfChildMatches(t, vizTree, "apples1")
	testIfChildMatches(t, vizTree, "appeasement")
	testIfChildMatches(t, vizTree, "appealing")
	testIfChildDoesNotMatches(t, vizTree, "date")
	testChildren(t, vizTree.child("date"), ":year")
	testIfChildDoesNotMatches(t, vizTree.child("date"), ":year")
	testChildren(t, vizTree.child("date").child(":year"), ":month", "month")
	testIfChildMatches(t, vizTree.child("date").child(":year"), ":month")
	testIfChildMatches(t, vizTree.child("date").child(":year"), "month")
	testChildren(t, vizTree.child("date").child(":year").child(":month"), ":post", "abc", "*post")
	testIfChildMatches(t, vizTree.child("date").child(":year").child(":month"), ":post")
	testIfChildMatches(t, vizTree.child("date").child(":year").child(":month"), "*post")
	testIfChildMatches(t, vizTree.child("date").child(":year").child(":month"), "abc")
	testIfChildMatches(t, vizTree, ":page")
	testChildren(t, vizTree.child(":page"), ":index")
	testIfChildMatches(t, vizTree.child(":page"), ":index")
	testIfChildDoesNotMatches(t, vizTree, "post")
	testChildren(t, vizTree.child("post"), ":post")
	testIfChildDoesNotMatches(t, vizTree.child("post"), ":post")
	testChildren(t, vizTree.child("post").child(":post"), "page")
	testIfChildDoesNotMatches(t, vizTree.child("post").child(":post"), "page")
	testChildren(t, vizTree.child("post").child(":post").child("page"), ":page")
	testIfChildMatches(t, vizTree.child("post").child(":post").child("page"), ":page")
	testIfChildMatches(t, vizTree, "plaster")
	testIfChildDoesNotMatches(t, vizTree, ":something")
	testChildren(t, vizTree.child(":something"), "abc", "def")
	testIfChildMatches(t, vizTree.child(":something"), "abc")
	testIfChildMatches(t, vizTree.child(":something"), "def")
	testIfChildDoesNotMatches(t, vizTree, "users")
	testChildren(t, vizTree.child("users"), ":pk", ":id")
	testIfChildDoesNotMatches(t, vizTree.child("users"), ":pk")
	testIfChildDoesNotMatches(t, vizTree.child("users"), ":id")
	testChildren(t, vizTree.child("users").child(":pk"), ":related")
	testChildren(t, vizTree.child("users").child(":id"), "updatePassword")
	testIfChildMatches(t, vizTree.child("users").child(":pk"), ":related")
	testIfChildMatches(t, vizTree.child("users").child(":id"), "updatePassword")
}

func testChildren(t *testing.T, tree *VizTree, expectedChildren ...string) {
	t.Log("Testing children of", tree.Path)
	children, err := childrenString(tree)
	if err != nil || children == nil {
		t.Fatalf("No children found")
		return
	}

	for _, child := range tree.Children {
		if len(child.leafWildcardNames) != 0 {
			t.Fatalf("leafWildcardNames different from nil")
			return
		}
	}
	sort.Strings(expectedChildren)
	sort.Strings(children)
	if !reflect.DeepEqual(children, expectedChildren) {
		t.Fatalf("Children  %v are different \n from expected children %v", children, expectedChildren)
	}
}
func testIfChildMatches(t *testing.T, tree *VizTree, childPath string) {
	t.Logf("Checking if child %v of %v has a matcher", childPath, tree.Path)
	child := tree.child(childPath)
	if child == nil {
		t.Fatalf("No children %v found in %v", childPath, tree.Path)
	}
	if !child.CanMatch {
		t.Fatalf("Child %v has no matcher", child.Path)
	}
}

func testIfChildDoesNotMatches(t *testing.T, tree *VizTree, childPath string) {
	t.Logf("Checking if child %v of %v has no matcher", childPath, tree.Path)
	child := tree.child(childPath)
	if child == nil {
		t.Fatalf("No children %v found in %v", childPath, tree.Path)
	}
	if child.CanMatch {
		t.Fatalf("Child %v should have no matcher", child.Path)
	}
}

func (n *VizTree) child(path string) *VizTree {
	return findNode(n.Children, path)
}

func childrenString(n *VizTree) (children []string, err error) {
	for _, child := range n.Children {
		children = append(children, child.Path)
	}
	return
}
