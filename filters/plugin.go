// All plugins must have a function named "InitFilter" with the following signature
//
//    func([]string) (filters.Spec, error)
//
// The parameters passed are all arguments for the plugin, i.e. everything after the first
// word from skipper's -filters parameter. E.g. when the -filters parameter is
// "myfilter datafile=/path/to/file foo=bar" the "myfilter" plugin will receive
//
//    []string{"datafile=/path/to/file", "foo=bar"}
//
// as arguments.
//
// The filter plugin implementation is responsible to parse the received arguments.
//
// An example plugin looks like
//
//     package main
//
//     import (
//          "github.com/zalando/skipper/filters"
//     )
//
//     type noopSpec struct{}
//     type noopFilter struct{}
//
//     func InitFilter(opts []string) (filters.Spec, error) {
//          return noopSpec{}, nil
//     }
//     func (s noopSpec) Name() string {
//			return "noop"
//     }
//     func (s noopSpec) CreateFilter(config []interface{}) (filters.Filter, error) {
//          return noopFilter{}, nil
//     }
//     func (f noopFilter) Request(filters.FilterContext) { }
//     func (f noopFilter) Response(filters.FilterContext) { }
//
//
// This should be built with
//
//    go build -buildmode=plugin -o noopfilter.so noop/noop.o
//
// and copied to the "filters" sub-directory of the directory given as -plugindir (by default, "./plugins").
// I.e. the module needs to go into ./plugins/filters/
//
// Then it can be loaded with -filters noopfilter as parameter to skipper.
package filters

import (
	"fmt"
	"path/filepath"
	"plugin"
)

// LoadPlugin loads the given filter plugin and returns an filter.Spec
func LoadPlugin(pluginDirs []string, opts []string) (Spec, error) {
	var impl string
	impl, opts = opts[0], opts[1:]

	var err error
	var mod *plugin.Plugin
	var pluginFile string
	for _, dir := range pluginDirs {
		pluginFile = filepath.Join(dir, impl+".so") // FIXME this is Linux and other ELF...
		mod, err = plugin.Open(pluginFile)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("open module %s: %s", pluginFile, err)
	}
	sym, err := mod.Lookup("InitFilter")
	if err != nil {
		return nil, fmt.Errorf("lookup module symbol failed for %s: %s", impl, err)
	}
	fn, ok := sym.(func([]string) (Spec, error))
	if !ok {
		return nil, fmt.Errorf("module %s's InitFilter function has wrong signature", impl)
	}
	spec, err := fn(opts)
	if err != nil {
		return nil, fmt.Errorf("module %s returned: %s", impl, err)
	}
	return spec, nil
}
