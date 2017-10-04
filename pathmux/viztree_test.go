package pathmux

import (
	"testing"
	"reflect"
	"sort"
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

	vizTree := (*vizNode)(NewVizTree(tree))
	if !vizTree.canMatch {
		t.Error("/ should match")
		return
	}
	testChildren(t, vizTree, []string{"i", "ima", "images", "images1", "images2", "apples", "apples1", "appeasement", "appealing", "app", "date", "plaster", ":page", "post", "users", ":something"})
	testIfChildMatch(t, vizTree, "i")
	testChildren(t, vizTree.child("i"), []string{":aaa"})
	testIfChildMatch(t, vizTree, "images")
	testChildren(t, vizTree.child("images"), []string{"abc.jpg", "*path", ":imgname"})
	testChildren(t, vizTree.child("images").child("abc.jpg"), []string{":size"})
	testIfChildMatch(t, vizTree, "images1")
	testChildren(t, vizTree.child("images1"), []string{"*path1"})
	testIfChildMatch(t, vizTree, "app")
	testChildren(t, vizTree.child("app"), []string{"les"})
	testChildren(t, vizTree.child(":something"), []string{"abc", "def"})
	testChildren(t, vizTree.child("users"), []string{":pk", ":id"})
	testChildren(t, vizTree.child("users").child(":pk"), []string{":related"})
	testChildren(t, vizTree.child("users").child(":id"), []string{"updatePassword"})
	testChildren(t, vizTree.child("post").child(":post").child("page"), []string{":page"})
	testChildren(t, vizTree.child("date"), []string{":year"})
	testChildren(t, vizTree.child("date").child(":year"), []string{":month", "month"})
	testChildren(t, vizTree.child("date").child(":year").child(":month"), []string{":post", "abc", "*post"})
}

func testChildren(t *testing.T, tree *vizNode, expectedChildren []string) {
	if t.Failed() {
		t.FailNow()
	}
	t.Log("Testing children of", tree.path)
	children, err := childrenString(tree)
	if err != nil || children == nil {
		t.Errorf("No children found")
		return
	}

	for i := 0; i < len(tree.children); i++ {
		if tree.children[i].leafWildcardNames != nil {
			t.Errorf("leafWildcardNames different from nil")
			return
		}
	}
	sort.Strings(expectedChildren)
	sort.Strings(children)
	if !reflect.DeepEqual(children, expectedChildren) {
		t.Errorf("Children  %v are different \n from expected children %v", children, expectedChildren)
		return
	}
}
func testIfChildMatch(t *testing.T, tree *vizNode, childPath string) {
	if t.Failed() {
		t.FailNow()
	}
	t.Logf("Looking for child %v of %v", childPath, tree.path)
	if tree.child(childPath) == nil {
		t.Errorf("No children %v found in %v", childPath, tree.path)
		return
	}
}

func childrenString(n *vizNode) (children []string, err error) {
	for i := 0; i < len(n.children); i++ {
		children = append(children, n.children[i].path)
	}
	return
}
