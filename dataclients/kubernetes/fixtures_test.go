package kubernetes

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
)

type fixtureSet struct {
	name           string
	resources      string
	eskip          string
	api            string
	defaultFilters string
	error          string
	log            string
}

// iterate over file names, looking for the ones with '.yaml' and '.eskip' extensions
// and same name, tolerating other files among the fixtures.
func rangeOverFixtures(t *testing.T, dir string, fs []os.FileInfo, test func(fixtureSet)) {
	// sort to ensure that the files belonging together by name are next to each other
	sort.Slice(fs, func(i, j int) bool {
		return fs[i].Name() < fs[j].Name()
	})

	var empty fixtureSet
	for len(fs) > 0 {
		var fixtures fixtureSet

		n := fs[0].Name()
		firstExt := filepath.Ext(n)
		fixtures.name = n[:len(n)-len(firstExt)]
		namePrefix := fixtures.name + "."
		for len(fs) > 0 {
			n := fs[0].Name()
			if !strings.HasPrefix(n, namePrefix) {
				break
			}

			switch filepath.Ext(n) {
			case ".yaml":
				fixtures.resources = filepath.Join(dir, n)
			case ".eskip":
				fixtures.eskip = filepath.Join(dir, n)
			case ".api":
				fixtures.api = filepath.Join(dir, n)
			case ".default-filters":
				fixtures.defaultFilters = filepath.Join(dir, n)
			case ".error":
				fixtures.error = filepath.Join(dir, n)
			case ".log":
				fixtures.log = filepath.Join(dir, n)
			}

			fs = fs[1:]
		}

		test(fixtures)
		fixtures = empty
	}
}

func matchOutput(matchFile, output string) error {
	b, err := ioutil.ReadFile(matchFile)
	if err != nil {
		return err
	}

	exps := strings.Split(string(b), "\n")
	lines := strings.Split(output, "\n")
	for _, e := range exps {
		rx := regexp.MustCompile(e)

		var found bool
		for _, l := range lines {
			if rx.MatchString(l) {
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("not matched: '%s'", e)
		}
	}

	return nil
}

func testFixture(t *testing.T, f fixtureSet) {
	var resources []io.Reader
	if f.resources != "" {
		r, err := os.Open(f.resources)
		if err != nil {
			t.Fatal(err)
		}

		defer r.Close()
		resources = append(resources, r)
	}

	var apiOptions testAPIOptions
	if f.api != "" {
		r, err := os.Open(f.api)
		if err != nil {
			t.Fatal(err)
		}

		defer r.Close()
		apiOptions, err = readAPIOptions(r)
		if err != nil {
			t.Fatal(err)
		}
	}

	a, err := newAPI(apiOptions, resources...)
	if err != nil {
		t.Fatal(err)
	}

	s := httptest.NewServer(a)
	defer s.Close()

	var logBuf bytes.Buffer
	// TODO: we should refactor the package to not use the global logger
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)
	defer func() {
		l := logBuf.String()
		if l != "" {
			t.Log("Captured logs:")
			t.Log(strings.TrimSpace(l))
		}
	}()

	c, err := New(Options{KubernetesURL: s.URL, DefaultFiltersDir: f.defaultFilters})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	routes, err := c.LoadAll()
	if f.eskip != "" {
		eskp, err := os.Open(f.eskip)
		if err != nil {
			t.Fatal(err)
		}

		defer eskp.Close()
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
			t.Logf("got:\n%s", eskip.String(eskip.CanonicalList(routes)...))
			t.Logf("expected:\n%s", eskip.String(eskip.CanonicalList(expectedRoutes)...))
		}
	}

	if f.error == "" && err != nil {
		t.Fatal(err)
	} else if f.error != "" {
		var msg string
		if err != nil {
			msg = err.Error()
		}

		if err := matchOutput(f.error, msg); err != nil {
			t.Errorf("Failed to match error: %v.", err)
		}
	}

	if f.log != "" {
		if err := matchOutput(f.log, logBuf.String()); err != nil {
			t.Errorf("Failed to match log: %v.", err)
		}
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

	defer d.Close()
	fs, err := d.Readdir(0)
	if err != nil {
		t.Fatal(err)
	}

	rangeOverFixtures(t, dir, fs, func(f fixtureSet) {
		t.Run(f.name, func(t *testing.T) {
			testFixture(t, f)
		})
	})
}
