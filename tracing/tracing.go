// Package tracing handles opentracing support for skipper
package tracing

import (
	"errors"
	"fmt"
	"path/filepath"
	"plugin"

	ot "github.com/opentracing/opentracing-go"
)

// Init sets up opentracing
func Init(pluginDir string, opts []string) (ot.Tracer, error) {
	if len(opts) == 0 {
		return nil, errors.New("no arguments passed")
	}
	var impl string
	impl, opts = opts[0], opts[1:]

	if impl == "noop" {
		return &ot.NoopTracer{}, nil
	}

	pluginFile := filepath.Join(pluginDir, impl+".so") // FIXME this is Linux and other ELF...
	mod, err := plugin.Open(pluginFile)
	if err != nil {
		return nil, fmt.Errorf("open module %s: %s", pluginFile, err)
	}
	sym, err := mod.Lookup("InitTracer")
	if err != nil {
		return nil, fmt.Errorf("lookup module symbol failed for %s: %s", impl, err)
	}
	fn, ok := sym.(func([]string) (ot.Tracer, error))
	if !ok {
		return nil, fmt.Errorf("module %s's InitTracer function has wrong signature", impl)
	}
	tracer, err := fn(opts)
	if err != nil {
		return nil, fmt.Errorf("module %s returned: %s", impl, err)
	}
	return tracer, nil
}
