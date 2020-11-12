package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/loadbalancer"
	"gopkg.in/yaml.v2"
)

func main() {
	var (
		name       string
		namespace  string
		hostString string
	)

	flag.StringVar(&name, "name", "", "name of the routegroup")
	flag.StringVar(&namespace, "namespace", "", "namespace of the routegroup")
	flag.StringVar(&hostString, "host", "", "hostname(s) to be used with the routegroup, comma separated if multiple")
	flag.Parse()
	args := flag.Args()
	hosts := strings.Split(hostString, ",")
	if len(hosts) == 1 && hosts[0] == "" {
		hosts = nil
	}

	var err error
	check := func() {
		if err != nil {
			log.Fatalln(err)
		}
	}

	input := []io.Reader{os.Stdin}
	if len(args) > 0 {
		input = nil
		if name == "" {
			name = filepath.Base(args[0])
			name = name[:len(name)-len(filepath.Ext(name))]
		}

		var f io.ReadCloser
		for i := 0; i < len(args); i++ {
			f, err = os.Open(args[i])
			check()
			defer f.Close()
			input = append(input, f)
		}
	}

	if name == "" {
		name = fmt.Sprintf(
			"%s_routegroup_%d",
			os.Getenv("USER"),
			time.Now().Unix(),
		)
	}

	var r []*eskip.Route
	for _, i := range input {
		var (
			b  []byte
			ri []*eskip.Route
		)

		b, err = ioutil.ReadAll(i)
		check()
		ri, err = eskip.Parse(string(b))
		check()
		r = append(r, ri...)
	}

	r = eskip.CanonicalList(r)
	unique := make(map[string]*eskip.Route)
	for _, ri := range r {
		unique[ri.Id] = ri
	}

	r = nil
	for _, ri := range unique {
		r = append(r, ri)
	}

	var rg definitions.RouteGroupItem
	rg.Metadata = &definitions.Metadata{}
	rg.Metadata.Name = name
	rg.Metadata.Namespace = namespace
	rg.Spec = &definitions.RouteGroupSpec{}
	rg.Spec.Hosts = hosts
	backendNames := make(map[string]string)
	backends := make(map[*eskip.Route]*definitions.SkipperBackend)
	nameBackend := func(key string) string {
		name, ok := backendNames[key]
		if !ok {
			name = fmt.Sprintf("backend%d", len(backendNames))
			backendNames[key] = name
		}

		return name
	}

	for _, ri := range r {
		b := &definitions.SkipperBackend{}
		b.Type = ri.BackendType
		switch ri.BackendType {
		case eskip.ShuntBackend:
			b.Name = nameBackend("<shunt>")
		case eskip.LoopBackend:
			b.Name = nameBackend("<loopback>")
		case eskip.DynamicBackend:
			b.Name = nameBackend("<dynamic>")
		case eskip.LBBackend:
			key := strings.Join(append([]string{ri.LBAlgorithm}, ri.LBEndpoints...), ",")
			b.Name = nameBackend(key)
			b.Algorithm, err = loadbalancer.AlgorithmFromString(ri.LBAlgorithm)
			check()
			b.Endpoints = ri.LBEndpoints
		default:
			b.Name = nameBackend(ri.Backend)
			var u *url.URL
			u, err = url.Parse(ri.Backend)
			check()
			if u.Scheme == "service" {
				b.Type = definitions.ServiceBackend
				b.ServiceName = u.Hostname()
				b.ServicePort, err = strconv.Atoi(u.Port())
				check()
			} else {
				b.Type = eskip.NetworkBackend
				b.Address = ri.Backend
			}
		}

		backends[ri] = b
	}

	for _, b := range backends {
		rg.Spec.Backends = append(rg.Spec.Backends, b)
	}

	for _, ri := range r {
		rs := &definitions.RouteSpec{}
		rs.Backends = []*definitions.BackendReference{{BackendName: backends[ri].Name}}
		for _, p := range ri.Predicates {
			var ok bool
			switch p.Name {
			case "Path":
				rs.Path, ok = p.Args[0].(string)
			case "PathSubtree":
				rs.PathSubtree, ok = p.Args[0].(string)
			case "PathRegexp":
				rs.PathRegexp, ok = p.Args[0].(string)
			case "Method":
				var m string
				m, ok = p.Args[0].(string)
				if ok {
					rs.Methods = []string{m}
				}
			case "Methods":
				var m string
				for _, a := range p.Args {
					m, ok = a.(string)
					if !ok {
						break
					}

					rs.Methods = append(rs.Methods, m)
				}
			default:
				rs.Predicates = append(rs.Predicates, p.String())
			}

			if !ok {
				log.Fatalln(
					"invalid predicate type in route: %s, predicate: %s",
					ri.Id,
					p.Name,
				)
			}
		}

		for _, f := range ri.Filters {
			rs.Filters = append(rs.Filters, f.String())
		}

		rg.Spec.Routes = append(rg.Spec.Routes, rs)
	}

	var b []byte
	b, err = yaml.Marshal(rg)
	check()
	_, err = os.Stdout.Write(b)
	check()
}
