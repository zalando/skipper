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
		logCmdLine       bool
		logEvents        bool
		maxBufferedSpans int

		grpcMaxMsgSize     = defaultGRPMaxMsgSize
		minReportingPeriod = lightstep.DefaultMinReportingPeriod
		maxReportingPeriod = lightstep.DefaultMaxReportingPeriod
	)

	componentName := defComponentName
	globalTags := make(map[string]string)

	for _, o := range opts {
		parts := strings.SplitN(o, "=", 2)
		switch parts[0] {
		case "component-name":
			if len(parts) > 1 {
				componentName = parts[1]
			}
		case "token":
			token = parts[1]
		case "grpc-max-msg-size":
			v, err := strconv.Atoi(parts[1])
			if err != nil {
				return lightstep.Options{}, fmt.Errorf("failed to parse %s as int grpc-max-msg-size: %v", parts[1], err)
			}
			grpcMaxMsgSize = v
		case "min-period":
			v, err := time.ParseDuration(parts[1])
			if err != nil {
				return lightstep.Options{}, fmt.Errorf("failed to parse %s as time.Duration min-period : %v", parts[1], err)
			}
			minReportingPeriod = v
		case "max-period":
			v, err := time.ParseDuration(parts[1])
			if err != nil {
				return lightstep.Options{}, fmt.Errorf("failed to parse %s as time.Duration max-period: %v", parts[1], err)
			}
			maxReportingPeriod = v
		case "tag":
			if len(parts) > 1 {
				tags := strings.SplitN(parts[1], "=", 2)
				if len(tags) != 2 {
					return lightstep.Options{}, fmt.Errorf("missing value for tag %s", tags[0])
				}
				globalTags[tags[0]] = tags[1]
			}
		case "collector":
			var err error
			var sport string

			host, sport, err = net.SplitHostPort(parts[1])
			if err != nil {
				return lightstep.Options{}, err
			}

			port, err = strconv.Atoi(sport)
			if err != nil {
				return lightstep.Options{}, fmt.Errorf("failed to parse %s as int: %v", sport, err)
			}
		case "cmd-line":
			cmdLine = parts[1]
			logCmdLine = true
		case "log-events":
			logEvents = true
		case "max-buffered-spans":
			var err error
			if maxBufferedSpans, err = strconv.Atoi(parts[1]); err != nil {
				return lightstep.Options{}, fmt.Errorf("failed to parse max buffered spans: %v", err)
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

	tags := map[string]interface{}{
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
			Host: host,
			Port: port,
		},
		UseGRPC:                     true,
		Tags:                        tags,
		MaxBufferedSpans:            maxBufferedSpans,
		GRPCMaxCallSendMsgSizeBytes: grpcMaxMsgSize,
		ReportingPeriod:             maxReportingPeriod,
		MinReportingPeriod:          minReportingPeriod,
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
