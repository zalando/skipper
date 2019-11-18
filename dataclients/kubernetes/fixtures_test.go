package kubernetes

import (
	"io/ioutil"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper/eskip"
)

// iterate over file names, looking for the ones with '.yaml' and '.eskip' extensions
// and same name, tolerating other files among the fixtures.
func rangeOverFixtures(t *testing.T, fs []os.FileInfo, test func(name, yamlPath, eskipPath string)) {
	// sort to ensure that the files belonging together by name are next to each other
	sort.Slice(fs, func(i, j int) bool {
		return fs[i].Name() < fs[j].Name()
	})

	for len(fs) > 0 {
		var resources, eskp string

		n := fs[0].Name()
		firstExt := filepath.Ext(n)
		fixtureName := n[:len(n)-len(firstExt)]
		namePrefix := fixtureName + "."
		for len(fs) > 0 {
			n := fs[0].Name()
			if !strings.HasPrefix(n, namePrefix) {
				break
			}

			switch filepath.Ext(n) {
			case ".yaml":
				resources = n
			case ".eskip":
				eskp = n
			}

			fs = fs[1:]
		}

		if resources != "" {
			test(fixtureName, resources, eskp)
		}
	}
}

func testFixture(t *testing.T, yamlPath, eskipPath string) {
	yml, err := os.Open(yamlPath)
	if err != nil {
		t.Fatal(err)
	}

	a, err := newAPI(yml)
	if err != nil {
		t.Fatal(err)
	}
	yml.Close()

	s := httptest.NewServer(a)
	defer s.Close()

	c, err := New(Options{KubernetesURL: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	routes, err := c.LoadAll()
	if err != nil {
		t.Fatal(err)
	}

	if eskipPath == "" {
		// in this case only testing that the load doesn't fail
		return
	}

	eskp, err := os.Open(eskipPath)
	if err != nil {
		t.Fatal(err)
	}

	b, err := ioutil.ReadAll(eskp)
	if err != nil {
		t.Fatal(err)
	}

	expectedRoutes, err := eskip.Parse(string(b))
	if err != nil {
		t.Fatal(err)
	}

	if !eskip.EqLists(routes, expectedRoutes) {
		t.Error("Failed to convert the resources to the right routes.")
		t.Logf("routes: %d, expected: %d", len(routes), len(expectedRoutes))
		t.Log(cmp.Diff(eskip.CanonicalList(routes), eskip.CanonicalList(expectedRoutes)))
	}
}

func testFixtures(t *testing.T, dir string) {
	if !filepath.IsAbs(dir) {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}

		dir = filepath.Join(wd, dir)
	}

	d, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	fs, err := d.Readdir(0)
	if err != nil {
		t.Fatal(err)
	}

	rangeOverFixtures(t, fs, func(name, yml, eskp string) {
		t.Run(name, func(t *testing.T) {
			yml := filepath.Join(dir, yml)
			if eskp != "" {
				eskp = filepath.Join(dir, eskp)
			}

			testFixture(t, yml, eskp)
		})
	})
}
