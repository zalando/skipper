package kubernetestest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/zalando/skipper/dataclients/kubernetes"
)

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
apiVersion: networking.k8s.io/v1
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
          service:
            name: foo
            port:
              name: main
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
---
apiVersion: v1
kind: Secret
metadata:
  labels:
    application: bar
  name: bar
  namespace: internal
data:
  tls.crt: foo
  tls.key: bar
type: kubernetes.io/tls
`

func getJSON(u string, o interface{}) error {
	rsp, err := http.Get(u)
	if err != nil {
		return err
	}

	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", rsp.StatusCode)
	}

	b, err := io.ReadAll(rsp.Body)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, o)
}

func TestTestAPI(t *testing.T) {
	kindListSpec, err := os.Open("testdata/kind-list.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer kindListSpec.Close()

	a, err := NewAPI(TestAPIOptions{},
		bytes.NewBufferString(testAPISpec1),
		bytes.NewBufferString(testAPISpec2),
		kindListSpec,
	)
	if err != nil {
		t.Fatal(err)
	}

	s := httptest.NewServer(a)
	defer s.Close()

	get := func(uri string, o interface{}) error {
		return getJSON(s.URL+uri, o)
	}

	check := func(t *testing.T, data map[string]interface{}, itemsLength int, kind string) {
		items, ok := data["items"].([]interface{})
		if !ok || len(items) != itemsLength {
			t.Fatalf("failed to get the right number of %s: expected %d, got %d", kind, itemsLength, len(items))
		}

		if itemsLength == 0 {
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
		if err := get(fmt.Sprintf(kubernetes.ServicesNamespaceFmt, namespace), &s); err != nil {
			t.Fatal(err)
		}

		check(t, s, 1, "Service")

		var i map[string]interface{}
		if err := get(fmt.Sprintf(kubernetes.IngressesV1NamespaceFmt, namespace), &i); err != nil {
			t.Fatal(err)
		}

		check(t, i, 0, "Ingress")

		var r map[string]interface{}
		if err := get(fmt.Sprintf("/apis/zalando.org/v1/namespaces/%s/routegroups", namespace), &r); err != nil {
			t.Fatal(err)
		}

		check(t, r, 1, "RouteGroup")

		var e map[string]interface{}
		if err := get(fmt.Sprintf(kubernetes.EndpointsNamespaceFmt, namespace), &e); err != nil {
			t.Fatal(err)
		}

		check(t, e, 1, "Endpoints")

		var sec map[string]interface{}
		if err := get(fmt.Sprintf(kubernetes.SecretsNamespaceFmt, namespace), &sec); err != nil {
			t.Fatal(err)
		}

		check(t, sec, 1, "Secret")
	})

	t.Run("without namespace", func(t *testing.T) {
		var s map[string]interface{}
		if err := get(kubernetes.ServicesClusterURI, &s); err != nil {
			t.Fatal(err)
		}

		check(t, s, 3, "Service")

		var i map[string]interface{}
		if err := get(kubernetes.IngressesV1ClusterURI, &i); err != nil {
			t.Fatal(err)
		}

		check(t, i, 1, "Ingress")

		var r map[string]interface{}
		if err := get("/apis/zalando.org/v1/routegroups", &r); err != nil {
			t.Fatal(err)
		}

		check(t, r, 2, "RouteGroup")

		var e map[string]interface{}
		if err := get(kubernetes.EndpointsClusterURI, &e); err != nil {
			t.Fatal(err)
		}

		check(t, e, 3, "Endpoints")

		var sec map[string]interface{}
		if err := get(kubernetes.SecretsClusterURI, &sec); err != nil {
			t.Fatal(err)
		}

		check(t, sec, 1, "Secret")
	})

	t.Run("kind: List", func(t *testing.T) {
		const namespace = "baz"

		var s map[string]interface{}
		if err := get(fmt.Sprintf(kubernetes.ServicesNamespaceFmt, namespace), &s); err != nil {
			t.Fatal(err)
		}

		check(t, s, 1, "Service")

		var i map[string]interface{}
		if err := get(fmt.Sprintf(kubernetes.IngressesNamespaceFmt, namespace), &i); err != nil {
			t.Fatal(err)
		}

		check(t, i, 0, "Ingress")

		var r map[string]interface{}
		if err := get(fmt.Sprintf("/apis/zalando.org/v1/namespaces/%s/routegroups", namespace), &r); err != nil {
			t.Fatal(err)
		}

		check(t, r, 1, "RouteGroup")

		var e map[string]interface{}
		if err := get(fmt.Sprintf(kubernetes.EndpointsNamespaceFmt, namespace), &e); err != nil {
			t.Fatal(err)
		}

		check(t, e, 1, "Endpoints")

		var sec map[string]interface{}
		if err := get(fmt.Sprintf(kubernetes.SecretsNamespaceFmt, namespace), &sec); err != nil {
			t.Fatal(err)
		}

		check(t, sec, 0, "Secret")
	})
}
