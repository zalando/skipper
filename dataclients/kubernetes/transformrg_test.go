package kubernetes

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	yaml2json "github.com/ghodss/yaml"
	"github.com/go-yaml/yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper/eskip"
)

type jsonClient []byte

type transformationTest struct {
	title           string
	routeGroupsPath string
	routesPath      string
}

func yaml2JSON(y []byte) ([]byte, error) {
	d := yaml.NewDecoder(bytes.NewBuffer(y))

	var yo []interface{}
	for {
		var o interface{}
		if err := d.Decode(&o); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		yo = append(yo, o)
	}

	yd := map[string]interface{}{
		"items": yo,
	}

	yb, err := yaml.Marshal(yd)
	if err != nil {
		return nil, err
	}

	return yaml2json.YAMLToJSON(yb)
}

func (c jsonClient) loadRouteGroups() ([]byte, error) {
	return []byte(c), nil
}

func collectFixtures() ([]transformationTest, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	fixtures := filepath.Join(wd, "fixtures/routegroups")
	d, err := os.Open(fixtures)
	if err != nil {
		return nil, err
	}

	fs, err := d.Readdir(0)
	if err != nil {
		return nil, err
	}

	sort.Slice(fs, func(i, j int) bool {
		return fs[i].Name() < fs[j].Name()
	})

	tests := make([]transformationTest, 0, len(fs)/2)
	for len(fs) > 1 {
		var t transformationTest
		name := fs[0].Name()
		firstExt := filepath.Ext(name)
		t.title = name[:len(name)-len(firstExt)]

		nameDot := t.title + "."
		for {
			name = fs[0].Name()
			if !strings.HasPrefix(name, nameDot) {
				break
			}

			switch filepath.Ext(name) {
			case ".yaml":
				t.routeGroupsPath = filepath.Join(fixtures, name)
			case ".eskip":
				t.routesPath = filepath.Join(fixtures, name)
			}

			fs = fs[1:]
		}

		if t.routeGroupsPath != "" && t.routesPath != "" {
			tests = append(tests, t)
		}
	}

	return tests, nil
}

func (test transformationTest) run(t *testing.T) {
	rgf, err := os.Open(test.routeGroupsPath)
	if err != nil {
		t.Fatal(err)
	}

	y, err := ioutil.ReadAll(rgf)
	if err != nil {
		t.Fatal(err)
	}

	j, err := yaml2JSON(y)
	if err != nil {
		t.Fatal(err)
	}

	dc, err := NewRouteGroupClient(RouteGroupsOptions{
		Kubernetes: Options{},
		apiClient:  jsonClient(j),
	})
	if err != nil {
		t.Fatal(err)
	}

	routes, err := dc.LoadAll()
	if err != nil {
		t.Fatal("Failed to convert route group document:", err)
	}

	rf, err := os.Open(test.routesPath)
	if err != nil {
		t.Fatal(err)
	}

	e, err := ioutil.ReadAll(rf)
	if err != nil {
		t.Fatal(err)
	}

	expectedRoutes, err := eskip.Parse(string(e))
	if err != nil {
		t.Fatal("Failed to parse expected routes:", err)
	}

	if !eskip.EqLists(routes, expectedRoutes) {
		t.Error("Failed to convert the route groups to the right routes.")
		t.Logf("routes: %d, expected: %d", len(routes), len(expectedRoutes))
		t.Log(cmp.Diff(eskip.CanonicalList(routes), eskip.CanonicalList(expectedRoutes)))
	}
}

func TestTransformRouteGroups(t *testing.T) {
	t.Skip()

	tests, err := collectFixtures()
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		t.Run(test.title, test.run)
	}
}
