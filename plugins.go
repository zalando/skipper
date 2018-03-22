package skipper

import (
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"strings"

	"github.com/zalando/skipper/filters"
)

func findAndLoadPlugins(o *Options) {
	found := make(map[string]string)

	for _, dir := range o.PluginDirs {
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if strings.HasSuffix(path, ".so") {
				name := filepath.Base(path)
				name = name[:len(name)-3] // strip suffix
				found[name] = path
				fmt.Printf("found plugin %s at %s\n", name, path)
			}
			return nil
		})
	}

	for _, fltr := range o.FilterPlugins {
		name := fltr[0]
		path, ok := found[name]
		if !ok {
			fmt.Printf("filter plugin %s not found in plugin dirs\n", name)
			continue
		}
		spec, err := LoadFilterPlugin(path, fltr[1:])
		if err != nil {
			fmt.Printf("failed to load plugin %s: %s\n", path, err)
			continue
		}
		o.CustomFilters = append(o.CustomFilters, spec)
		fmt.Printf("loaded plugin %s from %s\n", name, path)
		delete(found, name)
	}

	/*
		for _, pred := range o.PredicatePlugins {
			name := pred[0]
			path, ok := found[name]
			if !ok {
				fmt.Printf("predicate plugin %s not found in plugin dirs\n", name)
				continue
			}
			spec, err := LoadPredicatePlugin(path, fltr[1:])
			if err != nil {
				fmt.Printf("failed to load plugin %s: %s\n", path, err)
				continue
			}
			o.CustomPredicates = append(o.CustomPredicates, spec)
			delete(found, name)
		}
	*/

	for name, path := range found {
		mod, err := plugin.Open(path)
		if err != nil {
			fmt.Printf("open module %s from %s: %s", name, path, err)
			continue
		}
		if sym, err := mod.Lookup("InitFilter"); err == nil {
			spec, err := loadFilterPlugin(sym, path, []string{})
			if err != nil {
				fmt.Printf("filter module %s returned: %s", path, err)
				continue
			}
			o.CustomFilters = append(o.CustomFilters, spec)
			fmt.Printf("plugin %s loaded from %s\n", name, path)
			continue
		}
		/*
			if sym, err := mod.Lookup("InitPredicate"); err != nil {
				spec, err := loadPredicatePlugin(sym, path, []string{})
				if err != nil {
					fmt.Printf("predicate module %s returned: %s", path, err)
					continue
				}
				o.CustomPredicates = append(o.CustomPredicates, spec)
				continue
			}
			// same for DataClients ...
		*/
	}
}

func LoadFilterPlugin(path string, args []string) (filters.Spec, error) {
	mod, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open filter module %s: %s", path, err)
	}
	sym, err := mod.Lookup("InitFilter")
	if err != nil {
		return nil, fmt.Errorf("lookup module symbol failed for %s: %s", path, err)
	}
	return loadFilterPlugin(sym, path, args)
}

func loadFilterPlugin(sym plugin.Symbol, path string, args []string) (filters.Spec, error) {
	fn, ok := sym.(func([]string) (filters.Spec, error))
	if !ok {
		return nil, fmt.Errorf("module %s's InitFilter function has wrong signature", path)
	}
	spec, err := fn(args)
	if err != nil {
		return nil, fmt.Errorf("module %s returned: %s", path, err)
	}
	return spec, nil
}
