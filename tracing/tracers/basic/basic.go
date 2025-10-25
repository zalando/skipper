// Package basic is test infrastructure to enable tests to have a tracer.
package basic

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	basic "github.com/opentracing/basictracer-go"
	opentracing "github.com/opentracing/opentracing-go"
)

type CloseableTracer interface {
	opentracing.Tracer
	Close()
}

type basicTracer struct {
	opentracing.Tracer
	quit chan struct{}
	once sync.Once
}

func InitTracer(opts []string) (CloseableTracer, error) {
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

	quit := make(chan struct{})
	bt := &basicTracer{
		basic.NewWithOptions(basic.Options{
			DropAllLogs:    dropAllLogs,
			ShouldSample:   func(traceID uint64) bool { return traceID%sampleModulo == 0 },
			MaxLogsPerSpan: maxLogsPerSpan,
			Recorder:       recorder,
		}),
		quit,
		sync.Once{},
	}

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			rec := recorder.(*basic.InMemorySpanRecorder)
			spans := rec.GetSampledSpans()
			// Argh! we cannot lock it...
			rec.Reset()
			for _, span := range spans {
				fmt.Printf("SAMPLED=%#v\n", span)
			}

			select {
			case <-ticker.C:
			case <-quit:
				return
			}

		}
	}()

	return bt, nil
}

func missingArg(opt string) error {
	return fmt.Errorf("missing argument for %s option", opt)
}

func invalidArg(opt string, err error) error {
	return fmt.Errorf("invalid argument for %s option: %s", opt, err)
}

func (bt *basicTracer) Close() {
	bt.once.Do(func() {
		close(bt.quit)
	})
}
