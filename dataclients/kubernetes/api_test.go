package kubernetes

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	yaml2json "github.com/ghodss/yaml"
	"github.com/go-yaml/yaml"
)

type namespace struct {
	services    []byte
	ingresses   []byte
	routeGroups []byte
	endpoints   []byte
}

type api struct {
	namespaces map[string]namespace
	all        namespace
	pathRx     *regexp.Regexp
}

var errInvalidFixture = errors.New("invalid fixture")

func itemsJSON(b *[]byte, o []interface{}) error {
	items := map[string]interface{}{"items": o}

	// converting back to YAML, because we have YAMLToJSON() for bytes
	y, err := yaml.Marshal(items)
	if err != nil {
		return err
	}

	*b, err = yaml2json.YAMLToJSON(y)
	return err
}

func initNamespace(kinds map[string][]interface{}) (ns namespace, err error) {
	if err = itemsJSON(&ns.services, kinds["Service"]); err != nil {
		return
	}

	if err = itemsJSON(&ns.ingresses, kinds["Ingress"]); err != nil {
		return
	}

	if err = itemsJSON(&ns.routeGroups, kinds["RouteGroup"]); err != nil {
		return
	}

	if err = itemsJSON(&ns.endpoints, kinds["Endpoints"]); err != nil {
		return
	}

	return
}

func newAPI(specs ...io.Reader) (*api, error) {
	a := &api{
		namespaces: make(map[string]namespace),
		pathRx: regexp.MustCompile(
			"(/namespaces/([^/]+))?/(services|ingresses|routegroups|endpoints)",
		),
	}

	namespaces := make(map[string]map[string][]interface{})
	all := make(map[string][]interface{})

	for _, spec := range specs {
		d := yaml.NewDecoder(spec)
		for {
			var o map[string]interface{}
			if err := d.Decode(&o); err == io.EOF || o == nil {
				break
			} else if err != nil {
				return nil, err
			}

			kind, ok := o["kind"].(string)
			if !ok {
				return nil, errInvalidFixture
			}

			meta, ok := o["metadata"].(map[interface{}]interface{})
			if !ok {
				return nil, errInvalidFixture
			}

			namespace, ok := meta["namespace"]
			if !ok || namespace == "" {
				namespace = "default"
			} else {
				if _, ok := namespace.(string); !ok {
					return nil, errInvalidFixture
				}
			}

			ns := namespace.(string)
			if _, ok := namespaces[ns]; !ok {
				namespaces[ns] = make(map[string][]interface{})
			}

			namespaces[ns][kind] = append(namespaces[ns][kind], o)
			all[kind] = append(all[kind], o)
		}
	}

	for ns, kinds := range namespaces {
		var err error
		a.namespaces[ns], err = initNamespace(kinds)
		if err != nil {
			return nil, err
		}
	}

	var err error
	a.all, err = initNamespace(all)
	if err != nil {
		return nil, err
	}

	return a, nil
}

func (a *api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	parts := a.pathRx.FindStringSubmatch(r.URL.Path)
	if len(parts) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	ns := a.all
	if parts[2] != "" {
		ns = a.namespaces[parts[2]]
	}

	var b []byte
	switch parts[3] {
	case "services":
		b = ns.services
	case "ingresses":
		b = ns.ingresses
	case "routegroups":
		b = ns.routeGroups
	case "endpoints":
		b = ns.endpoints
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Write(b)
}

const testAPISpec1 = `
apiVersion: v1
kind: Service
metadata:
  labels:
    application: foo
  name: foo
  namespace: default
spec:
  ports:
  - name: main
    port: 80
    targetPort: 7272
  selector:
    application: foo
  type: ClusterIP
---
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  labels:
    application: foo
  name: foo
  namespace: default
spec:
  rules:
  - host: foo.example.org
    http:
      paths:
      - backend:
          serviceName: foo
          servicePort: main
---
apiVersion: v1
kind: Endpoints
metadata:
  labels:
    application: foo
  name: foo
  namespace: default
subsets:
- addresses:
  - ip: 10.0.0.0
    nodeName: node-10-1-0-0
  ports:
  - name: main
    port: 7272
    protocol: TCP
---
`

const testAPISpec2 = `
apiVersion: v1
kind: Service
metadata:
  labels:
    application: bar
  name: bar
  namespace: internal
spec:
  ports:
  - name: main
    port: 80
    targetPort: 7878
  selector:
    application: bar
  type: ClusterIP
---
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: foo
  namespace: internal
spec:
  hosts:
  - foo.example.org
  backends:
  - name: foo
    type: service
    serviceName: foo
    servicePort: 80
  routes:
  - pathSubtree: /
    backends:
    - backendName: foo
---
apiVersion: v1
kind: Endpoints
metadata:
  labels:
    application: bar
  name: bar
  namespace: internal
subsets:
- addresses:
  - ip: 10.0.0.2
    nodeName: node-10-1-0-2
  ports:
  - name: main
    port: 7878
    protocol: TCP
`

func TestTestAPI(t *testing.T) {
	a, err := newAPI(bytes.NewBufferString(testAPISpec1), bytes.NewBufferString(testAPISpec2))
	if err != nil {
		t.Fatal(err)
	}

	s := httptest.NewServer(a)
	defer s.Close()

	get := func(uri string, o interface{}) error {
		return getJSON(http.DefaultClient, "", s.URL+uri, o)
	}

	check := func(t *testing.T, data map[string]interface{}, length int, kind string) {
		items, ok := data["items"].([]interface{})
		if !ok || len(items) != length {
			t.Fatalf("failed to get the right number of items of: %s", kind)
		}

		if length == 0 {
			return
		}

		resource, ok := items[0].(map[string]interface{})
		if !ok || resource["kind"] != kind {
			t.Fatalf("failed to get the right resource: %s", kind)
		}
	}

	t.Run("with namespace", func(t *testing.T) {
		const namespace = "internal"

		var s map[string]interface{}
		if err := get(fmt.Sprintf(servicesNamespaceFmt, namespace), &s); err != nil {
			t.Fatal(err)
		}

		check(t, s, 1, "Service")

		var i map[string]interface{}
		if err := get(fmt.Sprintf(ingressesNamespaceFmt, namespace), &i); err != nil {
			t.Fatal(err)
		}

		check(t, i, 0, "Ingress")

		var r map[string]interface{}
		if err := get(fmt.Sprintf("/apis/zalando.org/v1/namespaces/%s/routegroups", namespace), &r); err != nil {
			t.Fatal(err)
		}

		check(t, r, 1, "RouteGroup")

		var e map[string]interface{}
		if err := get(fmt.Sprintf(endpointsNamespaceFmt, namespace), &e); err != nil {
			t.Fatal(err)
		}

		check(t, e, 1, "Endpoints")
	})

	t.Run("without namespace", func(t *testing.T) {
		var s map[string]interface{}
		if err := get(servicesClusterURI, &s); err != nil {
			t.Fatal(err)
		}

		check(t, s, 2, "Service")

		var i map[string]interface{}
		if err := get(ingressesClusterURI, &i); err != nil {
			t.Fatal(err)
		}

		check(t, i, 1, "Ingress")

		var r map[string]interface{}
		if err := get("/apis/zalando.org/v1/routegroups", &r); err != nil {
			t.Fatal(err)
		}

		check(t, r, 1, "RouteGroup")

		var e map[string]interface{}
		if err := get(endpointsClusterURI, &e); err != nil {
			t.Fatal(err)
		}

		check(t, e, 2, "Endpoints")
	})
}
