package kubernetestest

import (
	"bytes"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/secrets/certregistry"
	"gopkg.in/yaml.v2"
)

type fixtureSet struct {
	name           string
	resources      string
	eskip          string
	api            string
	kube           string
	defaultFilters string
	error          string
	log            string
}

type kubeOptionsParser struct {
	IngressV1                                      bool                              `yaml:"ingressv1"`
	EastWest                                       bool                              `yaml:"eastWest"`
	EastWestDomain                                 string                            `yaml:"eastWestDomain"`
	EastWestRangeDomains                           []string                          `yaml:"eastWestRangeDomains"`
	EastWestRangePredicates                        []*eskip.Predicate                `yaml:"eastWestRangePredicatesAppend"`
	HTTPSRedirect                                  bool                              `yaml:"httpsRedirect"`
	HTTPSRedirectCode                              int                               `yaml:"httpsRedirectCode"`
	DisableCatchAllRoutes                          bool                              `yaml:"disableCatchAllRoutes"`
	BackendNameTracingTag                          bool                              `yaml:"backendNameTracingTag"`
	OnlyAllowedExternalNames                       bool                              `yaml:"onlyAllowedExternalNames"`
	AllowedExternalNames                           []string                          `yaml:"allowedExternalNames"`
	IngressClass                                   string                            `yaml:"kubernetes-ingress-class"`
	KubernetesEnableEndpointSlices                 bool                              `yaml:"enable-kubernetes-endpointslices"`
	KubernetesEnableTLS                            bool                              `yaml:"kubernetes-enable-tls"`
	IngressesLabels                                map[string]string                 `yaml:"kubernetes-ingresses-label-selector"`
	ServicesLabels                                 map[string]string                 `yaml:"kubernetes-services-label-selector"`
	EndpointsLabels                                map[string]string                 `yaml:"kubernetes-endpoints-label-selector"`
	EndpointsliceLabels                            map[string]string                 `yaml:"kubernetes-endpointslice-label-selector"`
	ForceKubernetesService                         bool                              `yaml:"force-kubernetes-service"`
	BackendTrafficAlgorithm                        string                            `yaml:"backend-traffic-algorithm"`
	DefaultLoadBalancerAlgorithm                   string                            `yaml:"default-lb-algorithm"`
	KubernetesAnnotationPredicates                 []kubernetes.AnnotationPredicates `yaml:"kubernetesAnnotationPredicates"`
	KubernetesAnnotationFiltersAppend              []kubernetes.AnnotationFilters    `yaml:"kubernetesAnnotationFiltersAppend"`
	KubernetesEastWestRangeAnnotationPredicates    []kubernetes.AnnotationPredicates `yaml:"kubernetesEastWestRangeAnnotationPredicates"`
	KubernetesEastWestRangeAnnotationFiltersAppend []kubernetes.AnnotationFilters    `yaml:"kubernetesEastWestRangeAnnotationFiltersAppend"`
}

func baseNoExt(n string) string {
	e := filepath.Ext(n)
	return n[:len(n)-len(e)]
}

// iterate over file names, looking for the ones with '.yaml' and '.eskip' extensions
// and same name, tolerating other files among the fixtures.
func rangeOverFixtures(dir string, fs []os.FileInfo, test func(fixtureSet)) {
	// sort to ensure that the files belonging together by name are next to each other,
	// without extension
	sort.Slice(fs, func(i, j int) bool {
		ni := baseNoExt(fs[i].Name())
		nj := baseNoExt(fs[j].Name())
		return ni < nj
	})

	var empty fixtureSet
	for len(fs) > 0 {
		var fixtures fixtureSet

		fixtures.name = baseNoExt(fs[0].Name())
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
			case ".kube":
				fixtures.kube = filepath.Join(dir, n)
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
	b, err := os.ReadFile(matchFile)
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

func safeFileClose(t *testing.T, fd *os.File) {
	if err := fd.Close(); err != nil {
		t.Fatalf("Failed to close file: %v", err)
	}
}

func compileRegexps(s []string) ([]*regexp.Regexp, error) {
	r := make([]*regexp.Regexp, 0, len(s))
	for _, si := range s {
		ri, err := regexp.Compile(si)
		if err != nil {
			return nil, err
		}

		r = append(r, ri)
	}

	return r, nil
}

func testFixture(t *testing.T, f fixtureSet) {
	var resources []io.Reader
	if f.resources != "" {
		r, err := os.Open(f.resources)
		if err != nil {
			t.Fatal(err)
		}

		defer safeFileClose(t, r)
		resources = append(resources, r)
	}

	var apiOptions TestAPIOptions
	if f.api != "" {
		a, err := os.Open(f.api)
		if err != nil {
			t.Fatal(err)
		}

		defer safeFileClose(t, a)
		apiOptions, err = readAPIOptions(a)
		if err != nil {
			t.Fatal(err)
		}
	}

	a, err := NewAPI(apiOptions, resources...)
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

	var o kubernetes.Options
	var cr *certregistry.CertRegistry
	if f.kube != "" {
		ko, err := os.Open(f.kube)
		if err != nil {
			t.Fatal(err)
		}

		defer safeFileClose(t, ko)
		b, err := io.ReadAll(ko)
		if err != nil {
			t.Fatal(err)
		}

		var kop kubeOptionsParser
		if err := yaml.Unmarshal(b, &kop); err != nil {
			t.Fatal(err)
		}

		if kop.KubernetesEnableTLS {
			cr = certregistry.NewCertRegistry()
		}

		o.KubernetesEnableEndpointslices = kop.KubernetesEnableEndpointSlices
		o.KubernetesEnableEastWest = kop.EastWest
		o.KubernetesEastWestDomain = kop.EastWestDomain
		o.KubernetesEastWestRangeDomains = kop.EastWestRangeDomains
		o.KubernetesEastWestRangePredicates = kop.EastWestRangePredicates
		o.KubernetesAnnotationPredicates = kop.KubernetesAnnotationPredicates
		o.KubernetesAnnotationFiltersAppend = kop.KubernetesAnnotationFiltersAppend
		o.KubernetesEastWestRangeAnnotationPredicates = kop.KubernetesEastWestRangeAnnotationPredicates
		o.KubernetesEastWestRangeAnnotationFiltersAppend = kop.KubernetesEastWestRangeAnnotationFiltersAppend
		o.ProvideHTTPSRedirect = kop.HTTPSRedirect
		o.HTTPSRedirectCode = kop.HTTPSRedirectCode
		o.DisableCatchAllRoutes = kop.DisableCatchAllRoutes
		o.BackendNameTracingTag = kop.BackendNameTracingTag
		o.IngressClass = kop.IngressClass
		o.CertificateRegistry = cr
		o.IngressLabelSelectors = kop.IngressesLabels
		o.ServicesLabelSelectors = kop.ServicesLabels
		o.EndpointsLabelSelectors = kop.EndpointsLabels
		o.ForceKubernetesService = kop.ForceKubernetesService
		o.DefaultLoadBalancerAlgorithm = kop.DefaultLoadBalancerAlgorithm

		if kop.BackendTrafficAlgorithm != "" {
			o.BackendTrafficAlgorithm, err = kubernetes.ParseBackendTrafficAlgorithm(kop.BackendTrafficAlgorithm)
			if err != nil {
				t.Fatal(err)
			}
		}

		aen, err := compileRegexps(kop.AllowedExternalNames)
		if err != nil {
			t.Fatal(err)
		}

		o.OnlyAllowedExternalNames = kop.OnlyAllowedExternalNames
		o.AllowedExternalNames = aen
	}

	o.KubernetesURL = s.URL
	o.DefaultFiltersDir = f.defaultFilters
	c, err := kubernetes.New(o)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	routes, err := c.LoadAll()
	if f.api == "" && err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if f.eskip != "" {
		eskp, err := os.Open(f.eskip)
		if err != nil {
			t.Fatal(err)
		}

		defer safeFileClose(t, eskp)
		b, err := io.ReadAll(eskp)
		if err != nil {
			t.Fatal(err)
		}

		expectedRoutes, err := eskip.Parse(string(b))
		if err != nil {
			t.Fatal(err)
		}

		if !eskip.EqLists(routes, expectedRoutes) {
			sort.SliceStable(routes, func(i, j int) bool {
				return routes[i].Id < routes[j].Id
			})
			sort.SliceStable(expectedRoutes, func(i, j int) bool {
				return expectedRoutes[i].Id < expectedRoutes[j].Id
			})
			t.Error("Failed to convert the resources to the right routes.")
			t.Logf("routes: %d, expected: %d", len(routes), len(expectedRoutes))
			t.Logf("got:\n%s", eskip.String(eskip.CanonicalList(routes)...))
			t.Logf("expected:\n%s", eskip.String(eskip.CanonicalList(expectedRoutes)...))
			t.Logf("diff:\n%s", cmp.Diff(
				eskip.Print(eskip.PrettyPrintInfo{Pretty: true}, eskip.CanonicalList(expectedRoutes)...),
				eskip.Print(eskip.PrettyPrintInfo{Pretty: true}, eskip.CanonicalList(routes)...),
			))
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
			b, err := os.ReadFile(f.log)
			if err != nil {
				t.Fatal(err)
			}

			t.Errorf("Failed to match log: %v.", err)
			t.Logf("Got: %s", logBuf.String())
			t.Logf("Expected: %s", string(b))
		}
	}
}

func FixturesToTest(t *testing.T, dirs ...string) {
	for _, dir := range dirs {
		if !filepath.IsAbs(dir) {
			wd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}

			dir = filepath.Join(wd, dir)
		}
		println("dir:",dir)

		d, err := os.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer safeFileClose(t, d)

		fs, err := d.Readdir(0)
		if err != nil {
			t.Fatal(err)
		}

		rangeOverFixtures(dir, fs, func(f fixtureSet) {
			t.Run(f.name, func(t *testing.T) {
				testFixture(t, f)
			})
		})
	}
}
