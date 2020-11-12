package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
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

	rg, err := definitions.FromEskip(r)
	if err != nil {
		log.Fatalln(err)
	}

	rg.Metadata.Name = name
	rg.Metadata.Namespace = namespace
	rg.Spec.Hosts = hosts

	var b []byte
	b, err = yaml.Marshal(rg)
	check()
	_, err = os.Stdout.Write(b)
	check()
}
