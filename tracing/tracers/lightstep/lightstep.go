// Package lightstep integrates OpenTracing with [LightStep](https://github.com/lightstep/).
package lightstep

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	lightstep "github.com/lightstep/lightstep-tracer-go"
	opentracing "github.com/opentracing/opentracing-go"
)

const (
	defComponentName     = "skipper"
	defaultGRPMaxMsgSize = 16 * 1024 * 1000
)

func parseOptions(opts []string) (lightstep.Options, error) {
	var (
		port             int
		host, token      string
		cmdLine          string
		plaintext        bool
		logCmdLine       bool
		logEvents        bool
		maxBufferedSpans int
		maxLogKeyLen     int
		maxLogValueLen   int
		maxLogsPerSpan   int

		grpcMaxMsgSize     = defaultGRPMaxMsgSize
		minReportingPeriod = lightstep.DefaultMinReportingPeriod
		maxReportingPeriod = lightstep.DefaultMaxReportingPeriod
		propagators        = make(map[opentracing.BuiltinFormat]lightstep.Propagator)
		useGRPC            = true
	)

	componentName := defComponentName
	globalTags := make(map[string]string)

	defPropagator := lightstep.PropagatorStack{}
	defPropagator.PushPropagator(lightstep.LightStepPropagator)
	propagators[opentracing.HTTPHeaders] = defPropagator

	for _, o := range opts {
		key, val, _ := strings.Cut(o, "=")
		switch key {
		case "component-name":
			if val != "" {
				componentName = val
			}
		case "token":
			token = val
		case "grpc-max-msg-size":
			v, err := strconv.Atoi(val)
			if err != nil {
				return lightstep.Options{}, fmt.Errorf("failed to parse %s as int grpc-max-msg-size: %w", val, err)
			}
			grpcMaxMsgSize = v
		case "min-period":
			v, err := time.ParseDuration(val)
			if err != nil {
				return lightstep.Options{}, fmt.Errorf("failed to parse %s as time.Duration min-period : %w", val, err)
			}
			minReportingPeriod = v
		case "max-period":
			v, err := time.ParseDuration(val)
			if err != nil {
				return lightstep.Options{}, fmt.Errorf("failed to parse %s as time.Duration max-period: %w", val, err)
			}
			maxReportingPeriod = v
		case "tag":
			if val != "" {
				tag, tagVal, found := strings.Cut(val, "=")
				if !found {
					return lightstep.Options{}, fmt.Errorf("missing value for tag %s", val)
				}
				globalTags[tag] = tagVal
			}
		case "collector":
			var err error
			var sport string

			host, sport, err = net.SplitHostPort(val)
			if err != nil {
				return lightstep.Options{}, err
			}

			port, err = strconv.Atoi(sport)
			if err != nil {
				return lightstep.Options{}, fmt.Errorf("failed to parse %s as int: %w", sport, err)
			}
		case "plaintext":
			var err error
			plaintext, err = strconv.ParseBool(val)
			if err != nil {
				return lightstep.Options{}, fmt.Errorf("failed to parse %s as bool: %w", val, err)
			}
		case "cmd-line":
			cmdLine = val
			logCmdLine = true
		case "protocol":
			switch val {
			case "http":
				useGRPC = false
			case "grpc":
				useGRPC = true
			default:
				return lightstep.Options{}, fmt.Errorf("failed to parse protocol allowed 'http' or 'grpc', got: %s", val)
			}
		case "log-events":
			logEvents = true
		case "max-buffered-spans":
			var err error
			if maxBufferedSpans, err = strconv.Atoi(val); err != nil {
				return lightstep.Options{}, fmt.Errorf("failed to parse max buffered spans: %w", err)
			}
		case "max-log-key-len":
			var err error
			if maxLogKeyLen, err = strconv.Atoi(val); err != nil {
				return lightstep.Options{}, fmt.Errorf("failed to parse max log key length: %w", err)
			}
		case "max-log-value-len":
			var err error
			if maxLogValueLen, err = strconv.Atoi(val); err != nil {
				return lightstep.Options{}, fmt.Errorf("failed to parse max log value length: %w", err)
			}
		case "max-logs-per-span":
			var err error
			if maxLogsPerSpan, err = strconv.Atoi(val); err != nil {
				return lightstep.Options{}, fmt.Errorf("failed to parse max logs per span: %w", err)
			}
		case "propagators":
			if val != "" {
				prStack := lightstep.PropagatorStack{}
				prs := strings.SplitN(val, ",", 2)
				for _, pr := range prs {
					switch pr {
					case "lightstep", "ls":
						prStack.PushPropagator(lightstep.LightStepPropagator)
					case "b3":
						prStack.PushPropagator(lightstep.B3Propagator)
					default:
						return lightstep.Options{}, fmt.Errorf("unknown propagator `%v`", pr)
					}
				}
				propagators[opentracing.HTTPHeaders] = prStack
			}
		}
	}

	// Token is required.
	if token == "" {
		return lightstep.Options{}, errors.New("missing token= option")
	}

	// Set defaults.
	if host == "" {
		host = lightstep.DefaultGRPCCollectorHost
		port = lightstep.DefaultSecurePort
	}

	tags := map[string]any{
		lightstep.ComponentNameKey: componentName,
	}

	for k, v := range globalTags {
		tags[k] = v
	}
	if logCmdLine {
		tags[lightstep.CommandLineKey] = cmdLine
	}

	if logEvents {
		lightstep.SetGlobalEventHandler(createEventLogger())
	}

	if minReportingPeriod > maxReportingPeriod {
		return lightstep.Options{}, fmt.Errorf("wrong periods settings %s > %s", minReportingPeriod, maxReportingPeriod)
	}

	return lightstep.Options{
		AccessToken: token,
		Collector: lightstep.Endpoint{
			Host:      host,
			Port:      port,
			Plaintext: plaintext,
		},
		UseGRPC:                     useGRPC,
		Tags:                        tags,
		MaxBufferedSpans:            maxBufferedSpans,
		MaxLogKeyLen:                maxLogKeyLen,
		MaxLogValueLen:              maxLogValueLen,
		MaxLogsPerSpan:              maxLogsPerSpan,
		GRPCMaxCallSendMsgSizeBytes: grpcMaxMsgSize,
		ReportingPeriod:             maxReportingPeriod,
		MinReportingPeriod:          minReportingPeriod,
		Propagators:                 propagators,
	}, nil
}

func InitTracer(opts []string) (opentracing.Tracer, error) {
	lopt, err := parseOptions(opts)
	if err != nil {
		return nil, err
	}
	return lightstep.NewTracer(lopt), nil
}

func createEventLogger() lightstep.EventHandler {
	return func(event lightstep.Event) {
		if e, ok := event.(lightstep.ErrorEvent); ok {
			log.WithError(e).Warn("LightStep tracer received an error event")
		} else if e, ok := event.(lightstep.EventStatusReport); ok {
			log.WithFields(log.Fields{
				"duration":      e.Duration(),
				"sent_spans":    e.SentSpans(),
				"dropped_spans": e.DroppedSpans(),
			}).Debugf("Sent a report to the collectors")
		} else if _, ok := event.(lightstep.EventTracerDisabled); ok {
			log.Warn("LightStep tracer has been disabled")
		}
	}
}
