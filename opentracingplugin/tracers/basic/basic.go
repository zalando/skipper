package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	basic "github.com/opentracing/basictracer-go"
	opentracing "github.com/opentracing/opentracing-go"
)

func InitTracer(opts []string) (opentracing.Tracer, error) {
	fmt.Printf("DO NOT USE IN PRODUCTION\n")
	var dropAllLogs bool
	var sampleModulo uint64 = 1
	var maxLogsPerSpan int64 = 0
	var recorder basic.SpanRecorder = basic.NewInMemoryRecorder()
	var err error

	for _, o := range opts {
		parts := strings.SplitN(o, "=", 2)
		switch parts[0] {
		case "drop-all-logs":
			dropAllLogs = true

		case "sample-modulo":
			if len(parts) == 1 {
				return nil, missingArg(parts[0])
			}
			sampleModulo, err = strconv.ParseUint(parts[1], 10, 64)
			if err != nil {
				return nil, invalidArg(parts[0], err)
			}

		case "max-logs-per-span":
			if len(parts) == 1 {
				return nil, missingArg(parts[0])
			}
			maxLogsPerSpan, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return nil, invalidArg(parts[0], err)
			}

		case "recorder":
			if len(parts) == 1 {
				return nil, missingArg(parts[0])
			}
			switch parts[1] {
			case "in-memory":
				recorder = basic.NewInMemoryRecorder()
			default:
				return nil, fmt.Errorf("invalid recorder parameter")
			}
		}
	}
	go func() {
		for {
			rec := recorder.(*basic.InMemorySpanRecorder)
			spans := rec.GetSampledSpans()
			// Argh! we cannot lock it...
			rec.Reset()
			for _, span := range spans {
				fmt.Printf("SAMPLED=%#v\n", span)
			}
			time.Sleep(1 * time.Second)
		}
	}()

	return basic.NewWithOptions(basic.Options{
		DropAllLogs:    dropAllLogs,
		ShouldSample:   func(traceID uint64) bool { return traceID%sampleModulo == 0 },
		MaxLogsPerSpan: int(maxLogsPerSpan),
		Recorder:       recorder,
	}), nil
}

func missingArg(opt string) error {
	return fmt.Errorf("missing argument for %s option", opt)
}

func invalidArg(opt string, err error) error {
	return fmt.Errorf("invalid argument for %s option: %s", opt, err)
}
