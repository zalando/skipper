package kubernetes

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	log "github.com/sirupsen/logrus"
)

type testFileAPI struct {
	test         *testing.T
	testbasepath string
	server       *httptest.Server
	ingresses    *ingressList
	failNext     bool
}

func newTestFileAPI(t *testing.T) *testFileAPI {
	api := &testFileAPI{
		test:         t,
		testbasepath: "testdata/kube",
		ingresses:    &ingressList{},
	}

	api.server = httptest.NewServer(api)
	return api
}

func readFile(fpath string) ([]byte, error) {
	fd, err := os.OpenFile(fpath, os.O_RDONLY, 0444)
	if err != nil {
		return []byte(""), err
	}
	bts, err := ioutil.ReadAll(fd)
	if err != nil && err != io.EOF {
		return []byte(""), err
	}
	err = fd.Close()
	return bts, err
}

func (api *testFileAPI) getFilenames(basepath, ns string) []string {
	pattern := fmt.Sprintf("../../%s/%s/*.json", basepath, ns)
	filenames, err := filepath.Glob(pattern)
	if err != nil {
		api.test.Fatalf("Failed to glob files: %v", err)
	}
	return filenames
}

func (api *testFileAPI) getTestService(fpath string) (*service, bool) {

	fpath = "../../" + fpath + ".json"
	log.Debugf("getTestService: %s", fpath)

	b, err := readFile(fpath)
	if err != nil {
		log.Errorf("Failed to read file: %v", err)
		return nil, false
	}

	var result service
	err = json.Unmarshal(b, &result)
	if err != nil {
		log.Errorf("Failed to unmarshal: %v", err)
		return nil, false
	}

	if result.Meta.Namespace == "" {
		result.Meta.Namespace = "default"
	}

	// if log.GetLevel() == log.DebugLevel {
	// 	spew.Dump(result)
	// }

	return &result, true
}

func (api *testFileAPI) getTestIngresses(urlpath string) *ingressList {
	filenames := api.getFilenames(urlpath, "default")
	filenames = append(filenames, api.getFilenames(urlpath, "ns2")...)

	result := ingressList{Items: []*ingressItem{}}
	for _, fpath := range filenames {
		b, err := readFile(fpath)
		if err != nil {
			api.test.Fatalf("failed to readfile: %v", err)
		}

		var item ingressItem
		err = json.Unmarshal(b, &item)
		if err != nil {
			api.test.Fatalf("failed to unmarshal ingressItem: %s\n: %v", string(b), err)
		}
		if item.Metadata.Namespace == "" {
			item.Metadata.Namespace = "default"
		}
		result.Items = append(result.Items, &item)
	}
	log.Debugf("Found %d ingress definitions", len(result.Items))
	// if log.GetLevel() == log.DebugLevel {
	// 	spew.Dump(result)
	// }

	return &result
}

func (api *testFileAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if api.failNext {
		api.failNext = false
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	urlpath := api.testbasepath + r.URL.Path
	if r.URL.Path == ingressesURI {
		ingresses := api.getTestIngresses(urlpath)

		if err := respondJSON(w, ingresses); err != nil {
			api.test.Error(err)
		}

		return
	}

	s, ok := api.getTestService(urlpath)
	if !ok {
		s = &service{}
	}

	if err := respondJSON(w, s); err != nil {
		api.test.Error(err)
	}
}

func (api *testFileAPI) Close() {
	api.server.Close()
}

func TestMyIngress(t *testing.T) {
	api := newTestFileAPI(t)
	defer api.Close()
	dc, err := New(Options{KubernetesURL: api.server.URL})
	if err != nil {
		t.Error(err)
	}

	r, err := dc.LoadAll()
	if err != nil {
		t.Error(err)
		return
	}

	checkRoutes(t, r, map[string]string{
		"kube_default__app1__app_default_example_org____app1_svc":                                 "http://10.3.0.2:80",
		"kube_default__app1__app_default_example_org____app1_svc__lb_group":                       "",
		"kube_default__app_namedport_1__app_default_named_example_org____app_svc_named":           "http://10.3.0.3:80",
		"kube_default__app_namedport_1__app_default_named_example_org____app_svc_named__lb_group": "",
		"kube_ns2__app1__app1_ns2_example_org____app1_svc":                                        "http://10.3.1.3:80",
		"kube_ns2__app1__app1_ns2_example_org____app1_svc__lb_group":                              "",
	})
}

// ------------------------------------------------------------------

func TestMyHealthcheckInitial(t *testing.T) {
	api := newTestFileAPI(t)
	defer api.Close()

	t.Run("no healthcheck, empty", func(t *testing.T) {
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		r, err := dc.LoadAll()
		if err != nil {
			t.Error(err)
		}

		checkHealthcheck(t, r, false, false, false)
	})

	t.Run("no healthcheck", func(t *testing.T) {
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		r, err := dc.LoadAll()
		if err != nil {
			t.Error(err)
		}

		checkHealthcheck(t, r, false, false, false)
	})

	t.Run("no healthcheck, fail", func(t *testing.T) {
		api.failNext = true
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		_, err = dc.LoadAll()
		if err == nil {
			t.Error("failed to fail")
		}
	})

	t.Run("use healthcheck, empty", func(t *testing.T) {
		dc, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: true,
		})
		if err != nil {
			t.Error(err)
		}

		r, err := dc.LoadAll()
		if err != nil {
			t.Error(err)
		}

		checkHealthcheck(t, r, true, true, false)
	})

	t.Run("use healthcheck", func(t *testing.T) {
		dc, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: true,
		})
		if err != nil {
			t.Error(err)
		}

		r, err := dc.LoadAll()
		if err != nil {
			t.Error(err)
		}

		checkHealthcheck(t, r, true, true, false)
	})

	t.Run("use reverse healthcheck", func(t *testing.T) {
		dc, err := New(Options{
			KubernetesURL:          api.server.URL,
			ProvideHealthcheck:     true,
			ReverseSourcePredicate: true,
		})
		if err != nil {
			t.Error(err)
		}

		r, err := dc.LoadAll()
		if err != nil {
			t.Error(err)
		}

		checkHealthcheck(t, r, true, true, true)
	})
}

func TestMyHealthcheckUpdate(t *testing.T) {
	api := newTestFileAPI(t)
	defer api.Close()

	t.Run("no healthcheck, update fail", func(t *testing.T) {
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		dc.LoadAll()
		api.failNext = true

		r, d, err := dc.LoadUpdate()
		if err == nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, false, false, false)
		if len(d) != 0 {
			t.Error("unexpected delete")
		}
	})

	t.Run("use healthcheck, update fail", func(t *testing.T) {
		dc, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: true,
		})
		if err != nil {
			t.Error(err)
		}

		dc.LoadAll()
		api.failNext = true

		if _, _, err := dc.LoadUpdate(); err == nil {
			t.Error("failed to fail")
		}
	})

	t.Run("use healthcheck, update succeeds", func(t *testing.T) {
		dc, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: true,
		})
		if err != nil {
			t.Error(err)
		}

		dc.LoadAll()

		r, d, err := dc.LoadUpdate()
		if err != nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, true, true, false)
		if len(d) != 0 {
			t.Error("unexpected delete")
		}
	})

	t.Run("use healthcheck, update fails, gets fixed", func(t *testing.T) {
		dc, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: true,
		})
		if err != nil {
			t.Error(err)
		}

		dc.LoadAll()

		api.failNext = true
		dc.LoadUpdate()

		r, d, err := dc.LoadUpdate()
		if err != nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, true, true, false)
		if len(d) != 0 {
			t.Error("unexpected delete")
		}
	})
}

func TestMyHealthcheckReload(t *testing.T) {
	api := newTestFileAPI(t)
	defer api.Close()

	t.Run("no healthcheck, reload fail", func(t *testing.T) {
		dc, err := New(Options{KubernetesURL: api.server.URL})
		if err != nil {
			t.Error(err)
		}

		dc.LoadAll()
		api.failNext = true

		r, err := dc.LoadAll()
		if err == nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, false, false, false)
	})

	t.Run("use healthcheck, reload succeeds", func(t *testing.T) {
		dc, err := New(Options{
			KubernetesURL:      api.server.URL,
			ProvideHealthcheck: true,
		})
		if err != nil {
			t.Error(err)
		}

		dc.LoadAll()

		r, err := dc.LoadAll()
		if err != nil {
			t.Error("failed to fail")
		}

		checkHealthcheck(t, r, true, true, false)
		checkRoutes(t, r, map[string]string{
			healthcheckRouteID:                                                                        "",
			"kube_default__app1__app_default_example_org____app1_svc":                                 "http://10.3.0.2:80",
			"kube_default__app1__app_default_example_org____app1_svc__lb_group":                       "",
			"kube_default__app_namedport_1__app_default_named_example_org____app_svc_named":           "http://10.3.0.3:80",
			"kube_default__app_namedport_1__app_default_named_example_org____app_svc_named__lb_group": "",
			"kube_ns2__app1__app1_ns2_example_org____app1_svc":                                        "http://10.3.1.3:80",
			"kube_ns2__app1__app1_ns2_example_org____app1_svc__lb_group":                              "",
		})
	})
}
