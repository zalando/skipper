package main

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"gopkg.in/yaml.v2"
)

const rg200 = `apiVersion: zalando.org/v1
kind: RouteGroup
metadata: {}
spec:
  backends:
  - name: backend0
    type: shunt
  routes:
  - backends:
    - backendName: backend0
    filters:
    - status(200)
`

const rg200Name = `apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: foo
spec:
  backends:
  - name: backend0
    type: shunt
  routes:
  - backends:
    - backendName: backend0
    filters:
    - status(200)
`

const rg200NameNamespace = `apiVersion: zalando.org/v1
kind: RouteGroup
metadata:
  name: foo
  namespace: bar
spec:
  backends:
  - name: backend0
    type: shunt
  routes:
  - backends:
    - backendName: backend0
    filters:
    - status(200)
`

const rg200Hostnames = `apiVersion: zalando.org/v1
kind: RouteGroup
metadata: {}
spec:
  hosts:
  - www.example.org
  - www.example.com
  backends:
  - name: backend0
    type: shunt
  routes:
  - backends:
    - backendName: backend0
    filters:
    - status(200)
`

func TestRoutegroup(t *testing.T) {
	for _, test := range []struct {
		title     string
		routes    string
		filename  string
		hostnames []string
		name      string
		namespace string
		expect    string
		expectErr bool
	}{{
		title:     "invalid route syntax",
		routes:    `foo`,
		expectErr: true,
	}, {
		title:     "conversion of invalid route",
		routes:    `* -> "service://foo:not-a-port"`,
		expectErr: true,
	}, {
		title:  "only routes",
		routes: `* -> status(200) -> <shunt>`,
		expect: rg200,
	}, {
		title:     "with name and namespace",
		routes:    `* -> status(200) -> <shunt>`,
		name:      "foo",
		namespace: "bar",
		expect:    rg200NameNamespace,
	}, {
		title:    "only routes",
		routes:   `* -> status(200) -> <shunt>`,
		filename: "/tmp/foo.eskip",
		expect:   rg200Name,
	}, {
		title:     "with hostnames",
		routes:    `* -> status(200) -> <shunt>`,
		hostnames: []string{"www.example.org", "www.example.com"},
		expect:    rg200Hostnames,
	}} {
		t.Run(test.title, func(t *testing.T) {
			var args cmdArgs
			if test.filename == "" {
				args.in = &medium{typ: inline, eskip: test.routes}
			} else {
				args.in = &medium{typ: file, path: test.filename}
			}

			args.allMedia = []*medium{args.in}
			if len(test.hostnames) > 0 {
				args.allMedia = append(args.allMedia, &medium{
					typ:       hostnames,
					hostnames: test.hostnames,
				})
			}

			if test.name != "" {
				args.allMedia = append(args.allMedia, &medium{
					typ:            kubernetesName,
					kubernetesName: test.name,
				})
			}

			if test.namespace != "" {
				args.allMedia = append(args.allMedia, &medium{
					typ:                 kubernetesNamespace,
					kubernetesNamespace: test.namespace,
				})
			}

			var (
				buf bytes.Buffer
				err error
			)

			func() {
				defer func(original io.Writer) { stdout = original }(stdout)
				stdout = &buf
				withFile(test.filename, test.routes, func(*os.File) {
					err = routeGroupCmd(args)
				})
			}()

			if test.expectErr {
				if err == nil {
					t.Fatal("failed to fail")
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			var rg definitions.RouteGroupItem
			if err := yaml.Unmarshal(buf.Bytes(), &rg); err != nil {
				t.Fatal(err)
			}

			var expectRg definitions.RouteGroupItem
			if err := yaml.Unmarshal([]byte(test.expect), &expectRg); err != nil {
				t.Fatal(err)
			}

			if test.name == "" && test.filename == "" {
				if !strings.Contains(rg.Metadata.Name, os.Getenv("USER")) {
					t.Fatal("failed to generate the rotuegroup name")
				}

				rg.Metadata.Name = ""
			}

			if !reflect.DeepEqual(rg, expectRg) {
				t.Log("failed to generate the right routegroup")
				t.Log(
					cmp.Diff(
						rg,
						expectRg,
						cmp.AllowUnexported(definitions.SkipperBackend{}),
					),
				)
				t.Fatal()
			}
		})
	}
}
