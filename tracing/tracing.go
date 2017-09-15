// Package tracing handles opentracing support for skipper
//
// The tracers, except for "noop", are build as loadable modules. The modules must have
// an "InitTracer" function with the function signature
//
//    func([]string) (opentracing.Tracer, error)
//
// The parameters passed are all arguments for the module, i.e. everything after the first
// word from skipper's -opentracing parameter. E.g. when the -opentracing parmeter is
// "mytracer foo=bar token=xxx somename=bla:3" the "mytracer" plugin will receive
//
//    []string{"foo=bar", "token=xxx", "somename=bla:3"}
//
// as arguments.
//
// The tracer plugin is responsible for argument parsing.
//
// An example plugin looks like
//
//     package main
//
//     import (
//          instana "github.com/instana/golang-sensor"
//          opentracing "github.com/opentracing/opentracing-go"
//     )
//
//     func InitTracer(opts []string) (opentracing.Tracer, error) {
//          return instana.NewTracerWithOptions(&instana.Options{
//              Service:  "skipper",
//              LogLevel: instana.Error,
//          }), nil
//     }
//
// This needs to be build with
//
//    go build -buildmode=plugin -o instana.so ./instana/instana.go
//
// and copied to the directory given as -plugindir (by default, ".").
// Then it can be loaded with -opentracing "instana" as parameter to skipper.
package tracing

import (
	"errors"
	"fmt"
	"path/filepath"
	"plugin"

	ot "github.com/opentracing/opentracing-go"
)

// LoadPlugin loads the given opentracing plugin and returns an opentracing.Tracer
func LoadPlugin(pluginDir string, opts []string) (ot.Tracer, error) {
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
