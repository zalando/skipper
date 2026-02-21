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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
apiVersion: v1
kind: EndpointSlice
metadata:
  labels:
    application: foo
  name: foo-braj
  namespace: default
endpoints:
- addresses:
  - 10.0.0.0
  conditions:
    ready: true
    serving: true
    terminating: false
  zone: eu-central-1a
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
kind: EndpointSlice
metadata:
  labels:
    application: bar
  name: bar-kaiw
  namespace: internal
endpoints:
- addresses:
  - 10.0.0.2
  conditions:
    ready: true
    serving: true
    terminating: false
  zone: eu-central-1c
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

func getJSON(u string, o any) error {
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

func getField(o map[string]any, names ...string) any {
	name := names[0]
	if f, ok := o[name]; ok {
		if len(names) == 1 {
			return f
		}

		if m, ok := f.(map[string]any); ok {
			return getField(m, names[1:]...)
		} else {
			return nil
		}
	}
	return nil
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

	get := func(t *testing.T, uri string, o any) {
		t.Helper()
		err := getJSON(s.URL+uri, o)
		require.NoError(t, err)
	}

	check := func(t *testing.T, data map[string]any, itemsLength int, kind string) {
		t.Helper()
		items, ok := data["items"].([]any)
		if !ok || len(items) != itemsLength {
			t.Fatalf("failed to get the right number of %s: expected %d, got %d", kind, itemsLength, len(items))
		}

		if itemsLength == 0 {
			return
		}

		resource, ok := items[0].(map[string]any)
		if !ok {
			t.Fatalf("failed to get the right resource: %s", kind)
		}
		if resource["kind"] != kind {
			t.Fatalf("failed to get the right resource: %s != %s", resource["kind"], kind)
		}
	}

	t.Run("with namespace", func(t *testing.T) {
		const namespace = "internal"

		var s map[string]any
		get(t, fmt.Sprintf(kubernetes.ServicesNamespaceFmt, namespace), &s)
		check(t, s, 1, "Service")

		var i map[string]any
		get(t, fmt.Sprintf(kubernetes.IngressesV1NamespaceFmt, namespace), &i)
		check(t, i, 0, "Ingress")

		var r map[string]any
		get(t, fmt.Sprintf(kubernetes.RouteGroupsNamespaceFmt, namespace), &r)
		check(t, r, 1, "RouteGroup")

		var e map[string]any
		get(t, fmt.Sprintf(kubernetes.EndpointsNamespaceFmt, namespace), &e)
		check(t, e, 1, "Endpoints")

		var eps map[string]any
		get(t, fmt.Sprintf(kubernetes.EndpointSlicesNamespaceFmt, namespace), &eps)
		check(t, eps, 1, "EndpointSlice")

		var sec map[string]any
		get(t, fmt.Sprintf(kubernetes.SecretsNamespaceFmt, namespace), &sec)
		check(t, sec, 1, "Secret")
	})

	t.Run("without namespace", func(t *testing.T) {
		var s map[string]any
		get(t, kubernetes.ServicesClusterURI, &s)
		check(t, s, 3, "Service")

		var i map[string]any
		get(t, kubernetes.IngressesV1ClusterURI, &i)
		check(t, i, 1, "Ingress")

		var r map[string]any
		get(t, kubernetes.RouteGroupsClusterURI, &r)
		check(t, r, 2, "RouteGroup")

		var e map[string]any
		get(t, kubernetes.EndpointsClusterURI, &e)
		check(t, e, 3, "Endpoints")

		var eps map[string]any
		get(t, kubernetes.EndpointSlicesClusterURI, &eps)
		check(t, eps, 3, "EndpointSlice")

		var sec map[string]any
		get(t, kubernetes.SecretsClusterURI, &sec)
		check(t, sec, 1, "Secret")
	})

	t.Run("kind: List", func(t *testing.T) {
		const namespace = "baz"

		var s map[string]any
		get(t, fmt.Sprintf(kubernetes.ServicesNamespaceFmt, namespace), &s)
		check(t, s, 1, "Service")

		var i map[string]any
		get(t, fmt.Sprintf(kubernetes.IngressesV1NamespaceFmt, namespace), &i)
		check(t, i, 0, "Ingress")

		var r map[string]any
		get(t, fmt.Sprintf(kubernetes.RouteGroupsNamespaceFmt, namespace), &r)
		check(t, r, 1, "RouteGroup")

		var e map[string]any
		get(t, fmt.Sprintf(kubernetes.EndpointsNamespaceFmt, namespace), &e)
		check(t, e, 1, "Endpoints")

		var eps map[string]any
		get(t, fmt.Sprintf(kubernetes.EndpointSlicesNamespaceFmt, namespace), &eps)
		check(t, eps, 1, "EndpointSlice")

		var sec map[string]any
		get(t, fmt.Sprintf(kubernetes.SecretsNamespaceFmt, namespace), &sec)
		check(t, sec, 0, "Secret")
	})

	t.Run("resource by name", func(t *testing.T) {
		const namespace = "internal"

		var s map[string]any
		get(t, fmt.Sprintf(kubernetes.ServicesNamespaceFmt, namespace)+"/bar", &s)

		assert.Equal(t, "Service", getField(s, "kind"))
		assert.Equal(t, namespace, getField(s, "metadata", "namespace"))
		assert.Equal(t, "bar", getField(s, "metadata", "name"))

		var e map[string]any
		get(t, fmt.Sprintf(kubernetes.EndpointsNamespaceFmt, namespace)+"/bar", &e)

		assert.Equal(t, "Endpoints", getField(e, "kind"))
		assert.Equal(t, namespace, getField(e, "metadata", "namespace"))
		assert.Equal(t, "bar", getField(e, "metadata", "name"))
	})

	t.Run("resource name does not exist", func(t *testing.T) {
		const namespace = "internal"

		var o map[string]any
		err := getJSON(s.URL+fmt.Sprintf(kubernetes.ServicesNamespaceFmt, namespace)+"/does-not-exist", &o)

		assert.EqualError(t, err, "unexpected status code: 404")
	})
}
