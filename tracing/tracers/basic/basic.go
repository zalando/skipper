package basic

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
	var (
		dropAllLogs    bool
		sampleModulo   uint64             = 1
		maxLogsPerSpan                    = 0
		recorder       basic.SpanRecorder = basic.NewInMemoryRecorder()
		err            error
	)

	for _, o := range opts {
		k, v, _ := strings.Cut(o, "=")
		switch k {
		case "drop-all-logs":
			dropAllLogs = true

		case "sample-modulo":
			if v == "" {
				return nil, missingArg(k)
			}
			sampleModulo, err = strconv.ParseUint(v, 10, 64)
			if err != nil {
				return nil, invalidArg(k, err)
			}

		case "max-logs-per-span":
			if v == "" {
				return nil, missingArg(k)
			}
			maxLogsPerSpan, err = strconv.Atoi(v)
			if err != nil {
				return nil, invalidArg(k, err)
			}

		case "recorder":
			if v == "" {
				return nil, missingArg(k)
			}
			switch v {
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
		MaxLogsPerSpan: maxLogsPerSpan,
		Recorder:       recorder,
	}), nil
}

func missingArg(opt string) error {
	return fmt.Errorf("missing argument for %s option", opt)
}

func invalidArg(opt string, err error) error {
	return fmt.Errorf("invalid argument for %s option: %s", opt, err)
}
