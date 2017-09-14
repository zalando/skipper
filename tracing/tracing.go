// Package tracing handles opentracing support for skipper
package tracing

import (
	"errors"
	"fmt"
	"path/filepath"
	"plugin"

	ot "github.com/opentracing/opentracing-go"
)

var (
	// ErrUnsupportedTracer is returned when an unsupported opentracing
	// implementation was requested as tracer
	ErrUnsupportedTracer error = errors.New("invalid argument, not a supported tracer")
	// ErrMissingArguments is returned when an empty list is passed to Init()
	ErrMissingArguments error = errors.New("no arguments passed")
)

// Tracer is required to be implemented by any module
type Tracer interface {
	InitTracer(opts []string) (ot.Tracer, error)
}

// Init sets up opentracing
func Init(pluginDir string, opts []string) error {
	if len(opts) == 0 {
		return ErrMissingArguments
	}
	var impl string
	impl, opts = opts[0], opts[1:]

	var tracer ot.Tracer
	if impl == "noop" {
		tracer = &ot.NoopTracer{}
	} else {
		mod, err := plugin.Open(filepath.Join(pluginDir, impl+".so")) // FIXME this is Linux and other ELF...
		if err != nil {
			return fmt.Errorf("open module %s: %s", impl, err)
		}
		tracerSym, err := mod.Lookup("Tracer")
		if err != nil {
			return fmt.Errorf("check module symbols %s: %s", impl, err)
		}
		pluggedTracer, ok := tracerSym.(Tracer)
		if !ok {
			return fmt.Errorf("module %s does not implement Tracer", impl)
		}
		tracer, err = pluggedTracer.InitTracer(opts)
		if err != nil {
			return fmt.Errorf("module %s returned: %s", impl, err)
		}
	}
	ot.SetGlobalTracer(tracer)
	return nil
}
