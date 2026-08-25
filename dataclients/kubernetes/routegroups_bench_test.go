package kubernetes_test

import (
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes"
	"github.com/zalando/skipper/dataclients/kubernetes/kubernetestest"
)

// BenchmarkRouteGroupsLoadAll drives the full LoadAll path (fetch, validate,
// convert) over many RouteGroups whose routes carry several predicates and
// filters, exercising the eskip parse that validation and conversion share.
func BenchmarkRouteGroupsLoadAll(b *testing.B) {
	const routeGroups = 200

	var sb strings.Builder
	sb.WriteString(`apiVersion: v1
kind: Service
metadata:
  name: svc
  namespace: default
spec:
  ports:
  - port: 80
    protocol: TCP
    targetPort: 80
  type: ClusterIP
---
apiVersion: v1
kind: Endpoints
metadata:
  name: svc
  namespace: default
subsets:
- addresses:
  - ip: 10.2.1.8
  - ip: 10.2.1.16
  ports:
  - port: 80
`)

	for i := range routeGroups {
		fmt.Fprintf(&sb, `---
apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: rg-%d
  namespace: default
spec:
  hosts:
  - rg%d.example.org
  backends:
  - name: svc
    type: service
    serviceName: svc
    servicePort: 80
  defaultBackends:
  - backendName: svc
  routes:
  - path: /a
    methods:
    - GET
    - POST
    predicates:
    - Header("X-Test", "foo")
    - Cookie("session", "^v")
    filters:
    - setRequestHeader("X-A", "1")
    - setResponseHeader("X-B", "2")
    - inlineContent("hello")
  - path: /b
    predicates:
    - QueryParam("q", "^v$")
    filters:
    - status(200)
`, i, i)
	}

	spec := sb.String()

	a, err := kubernetestest.NewAPI(kubernetestest.TestAPIOptions{}, strings.NewReader(spec))
	if err != nil {
		b.Fatal(err)
	}
	s := httptest.NewServer(a)
	defer s.Close()

	c, err := kubernetes.New(kubernetes.Options{KubernetesURL: s.URL})
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, err := c.LoadAll(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRouteGroupsLoadAllDump drives the same LoadAll path over a real
// cluster dump instead of synthetic RouteGroups. Create the dump with:
//
//	kubectl get routegroups.zalando.org -A -o json > rgs.json
//
// then run:
//
//	SKIPPER_RG_DUMP=$PWD/rgs.json go test -run=NONE \
//	  -bench=BenchmarkRouteGroupsLoadAllDump -benchmem ./dataclients/kubernetes/
//
// The dump can carry tokens/PII in filter args; keep it local, never commit it.
func BenchmarkRouteGroupsLoadAllDump(b *testing.B) {
	path := os.Getenv("SKIPPER_RG_DUMP")
	if path == "" {
		b.Skip("set SKIPPER_RG_DUMP=/path/to/rgs.json (kubectl get routegroups.zalando.org -A -o json)")
	}

	// A real dump can reference Services that no longer exist, and skipper logs
	// each one. Those logs interleave with the benchmark output, so discard them.
	logOut := log.StandardLogger().Out
	log.SetOutput(io.Discard)
	defer log.SetOutput(logOut)

	f, err := os.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	a, err := kubernetestest.NewAPI(kubernetestest.TestAPIOptions{}, f)
	if err != nil {
		b.Fatal(err)
	}
	s := httptest.NewServer(a)
	defer s.Close()

	c, err := kubernetes.New(kubernetes.Options{KubernetesURL: s.URL})
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, err := c.LoadAll(); err != nil {
			b.Fatal(err)
		}
	}
}
