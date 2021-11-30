package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"gopkg.in/yaml.v2"
)

// command executed for routegroup.
func routeGroupCmd(a cmdArgs) error {
	var (
		name      string
		namespace string
		hs        []string
	)

	for _, m := range a.allMedia {
		switch m.typ {
		case kubernetesName:
			name = m.kubernetesName
		case kubernetesNamespace:
			namespace = m.kubernetesNamespace
		case hostnames:
			hs = m.hostnames
		case file:
			if name == "" {
				name = filepath.Base(m.path)
				name = name[:len(name)-len(filepath.Ext(name))]
			}
		}
	}

	if name == "" {
		name = fmt.Sprintf(
			"%s_routegroup_%d",
			os.Getenv("USER"),
			time.Now().Unix(),
		)
	}

	lr, err := loadRoutes(a.in)
	if err != nil {
		return err
	}

	if len(lr.parseErrors) > 0 {
		return errInvalidRouteExpression
	}

	rg, err := definitions.FromEskip(lr.routes)
	if err != nil {
		return err
	}

	if rg.Metadata == nil {
		rg.Metadata = &definitions.Metadata{}
	}

	rg.Metadata.Name = name
	rg.Metadata.Namespace = namespace

	if rg.Spec == nil {
		rg.Spec = &definitions.RouteGroupSpec{}
	}

	rg.Spec.Hosts = hs
	b, err := yaml.Marshal(rg)
	if err != nil {
		return err
	}

	if _, err := stdout.Write(b); err != nil {
		return err
	}

	return nil
}
