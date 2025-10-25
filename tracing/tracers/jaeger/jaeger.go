// Package jaeger integrates OpenTracing with [Jaeger](https://www.jaegertracing.io/).
package jaeger

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-lib/metrics/prometheus"
)

const (
	defServiceName = "skipper"
)

func parseOptions(opts []string) (*config.Configuration, error) {
	useRPCMetrics := false
	serviceName := defServiceName
	var err error
	var samplerParam float64
	var samplerType string
	var samplerURL string
	var localAgent string
	var reporterQueue int
	var reporterInterval time.Duration
	var globalTags []opentracing.Tag

	for _, o := range opts {
		k, v, _ := strings.Cut(o, "=")
		switch k {
		case "service-name":
			if v != "" {
				serviceName = v
			}

		case "use-rpc-metrics":
			useRPCMetrics = true

		case "sampler-type":
			if v == "" {
				return nil, missingArg(k)
			}
			samplerType, v, _ = strings.Cut(v, ":")
			switch samplerType {
			case "const":
				samplerParam = 1.0
			case "probabilistic", "rateLimiting", "remote":
				if v == "" {
					return nil, missingArg(k)
				}
				samplerParam, err = strconv.ParseFloat(v, 64)
				if err != nil {
					return nil, invalidArg(v, err)
				}
			default:
				return nil, invalidArg(k, errors.New("invalid sampler type"))
			}
		case "sampler-url":
			if v == "" {
				return nil, missingArg(k)
			}
			samplerURL = v

		case "reporter-queue":
			if v == "" {
				return nil, missingArg(k)
			}
			reporterQueue, _ = strconv.Atoi(v)
		case "reporter-interval":
			if v == "" {
				return nil, missingArg(k)
			}
			reporterInterval, err = time.ParseDuration(v)
			if err != nil {
				return nil, invalidArg(v, err)
			}
		case "local-agent":
			if v == "" {
				return nil, missingArg(k)
			}
			localAgent = v
		case "tag":
			if v != "" {
				k, v, _ := strings.Cut(v, "=")
				if v == "" {
					return nil, fmt.Errorf("missing value for tag %s", k)
				}

				globalTags = append(globalTags, opentracing.Tag{Key: k, Value: v})
			}
		}
	}

	conf := &config.Configuration{
		ServiceName: serviceName,
		Disabled:    false,
		Sampler: &config.SamplerConfig{
			Type:              samplerType,
			Param:             samplerParam,
			SamplingServerURL: samplerURL,
		},
		Reporter: &config.ReporterConfig{
			QueueSize:           reporterQueue,
			BufferFlushInterval: reporterInterval,
			LocalAgentHostPort:  localAgent,
		},
		RPCMetrics: useRPCMetrics,
		Tags:       globalTags,
	}
	return conf, nil
}

func InitTracer(opts []string) (opentracing.Tracer, error) {
	conf, err := parseOptions(opts)
	if err != nil {
		return nil, err
	}

	metricsFactory := prometheus.New()
	tracer, _, err := conf.NewTracer(config.Metrics(metricsFactory))
	return tracer, err
}

func missingArg(opt string) error {
	return fmt.Errorf("missing argument for %s option", opt)
}

func invalidArg(opt string, err error) error {
	return fmt.Errorf("invalid argument for %s option: %s", opt, err)
}
