// Package instana integrates OpenTracing with provider [Instana (IBM)](https://www.instana.com/) for skipper.
package instana

import (
	"strings"

	instana "github.com/instana/go-sensor"
	opentracing "github.com/opentracing/opentracing-go"
)

const (
	defServiceName = "skipper"
)

func InitTracer(opts []string) (opentracing.Tracer, error) {
	serviceName := defServiceName

	for _, o := range opts {
		k, v, _ := strings.Cut(o, "=")
		switch k {
		case "service-name":
			if v != "" {
				serviceName = v
			}
		}
	}

	return instana.NewTracerWithOptions(&instana.Options{
		Service:  serviceName,
		LogLevel: instana.Error,
	}), nil
}
