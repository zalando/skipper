package lightstep

import (
	"errors"
	"fmt"
	"github.com/zalando/skipper/logging"
	"net"
	"strconv"
	"strings"

	lightstep "github.com/lightstep/lightstep-tracer-go"
	opentracing "github.com/opentracing/opentracing-go"
)

const (
	defComponentName = "skipper"
)

func InitTracer(opts []string) (opentracing.Tracer, error) {
	componentName := defComponentName
	var port int
	var host, token string
	var cmdLine string
	var logCmdLine, logEvents bool
	var maxBufferedSpans int
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
		case "tag":
			if len(parts) > 1 {
				tags := strings.SplitN(parts[1], "=", 2)
				if len(tags) != 2 {
					return nil, fmt.Errorf("missing value for tag %s", tags[0])
				}
				globalTags[tags[0]] = tags[1]
			}
		case "collector":
			var err error
			var sport string

			host, sport, err = net.SplitHostPort(parts[1])
			if err != nil {
				return nil, err
			}

			port, err = strconv.Atoi(sport)
			if err != nil {
				return nil, fmt.Errorf("failed to parse %s as int: %v", sport, err)
			}
		case "cmd-line":
			cmdLine = parts[1]
			logCmdLine = true
		case "log-events":
			logEvents = true
		case "max-buffered-spans":
			var err error
			if maxBufferedSpans, err = strconv.Atoi(parts[1]); err != nil {
				return nil, fmt.Errorf("failed to parse max buffered spans: %v", err)
			}
		}
	}

	// Token is required.
	if token == "" {
		return nil, errors.New("missing token= option")
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
		logger := &logging.DefaultLog{}
		lightstep.SetGlobalEventHandler(createEventLogger(logger))
	}

	return lightstep.NewTracer(lightstep.Options{
		AccessToken: token,
		Collector: lightstep.Endpoint{
			Host: host,
			Port: port,
		},
		UseGRPC:          true,
		Tags:             tags,
		MaxBufferedSpans: maxBufferedSpans,
	}), nil
}

func createEventLogger(logger logging.Logger) lightstep.EventHandler {
	return func(event lightstep.Event) {
		if e, ok := event.(lightstep.ErrorEvent); ok {
			logger.Warn("LightStep tracer received an error event", e)
		} else if e, ok := event.(lightstep.EventStatusReport); ok {
			logger.Debugf("Sent a report to the collectors in %d ms. Sent spans: %d - dropped spans: %d", e.Duration(), e.SentSpans(), e.DroppedSpans())
		} else if _, ok := event.(lightstep.EventTracerDisabled); ok {
			logger.Warn("LightStep tracer has been disabled")
		}
	}
}
