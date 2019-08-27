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

func InitTracer(opts []string) (opentracing.Tracer, error) {
	metricsFactory := prometheus.New()

	useRPCMetrics := false
	serviceName := defServiceName
	var err error
	var samplerParam float64
	var samplerType string
	var samplerURL string
	var localAgent string
	var reporterQueue int64
	var reporterInterval time.Duration
	var globalTags []opentracing.Tag

	for _, o := range opts {
		parts := strings.SplitN(o, "=", 2)
		switch parts[0] {
		case "service-name":
			if len(parts) > 1 {
				serviceName = parts[1]
			}

		case "use-rpc-metrics":
			useRPCMetrics = true

		case "sampler-type":
			if len(parts) == 1 {
				return nil, missingArg(parts[0])
			}
			samplerValue := parts[1]
			parts = strings.SplitN(samplerValue, ":", 2)
			samplerType = parts[0]
			switch samplerType {
			case "const":
				samplerParam = 1.0
			case "probabilistic", "rateLimiting", "remote":
				if len(parts) == 1 {
					return nil, missingArg(parts[1])
				}
				samplerParam, err = strconv.ParseFloat(parts[1], 64)
				if err != nil {
					return nil, invalidArg(parts[1], err)
				}
			default:
				return nil, invalidArg(parts[0], errors.New("invalid sampler type"))
			}
		case "sampler-url":
			if len(parts) == 1 {
				return nil, missingArg(parts[0])
			}
			samplerURL = parts[1]

		case "reporter-queue":
			if len(parts) == 1 {
				return nil, missingArg(parts[0])
			}
			reporterQueue, _ = strconv.ParseInt(parts[1], 10, 64)
		case "reporter-interval":
			if len(parts) == 1 {
				return nil, missingArg(parts[0])
			}
			reporterInterval, err = time.ParseDuration(parts[1])
			if err != nil {
				return nil, invalidArg(parts[1], err)
			}
		case "local-agent":
			if len(parts) == 1 {
				return nil, missingArg(parts[0])
			}
			localAgent = parts[1]
		case "tag":
			if len(parts) > 1 {
				kv := strings.SplitN(parts[1], "=", 2)
				if len(kv) != 2 {
					return nil, fmt.Errorf("missing value for tag %s", kv[0])
				}

				globalTags = append(globalTags, opentracing.Tag{Key: kv[0], Value: kv[1]})
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
			QueueSize:           int(reporterQueue),
			BufferFlushInterval: reporterInterval,
			LocalAgentHostPort:  localAgent,
		},
		RPCMetrics: useRPCMetrics,
		Tags:       globalTags,
	}
	tracer, _, err := conf.NewTracer(config.Metrics(metricsFactory))
	return tracer, err
}

func missingArg(opt string) error {
	return fmt.Errorf("missing argument for %s option", opt)
}

func invalidArg(opt string, err error) error {
	return fmt.Errorf("invalid argument for %s option: %s", opt, err)
}
