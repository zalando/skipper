package skipper

import (
	"fmt"

	"os"
	"path/filepath"
	"plugin"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/routing"
)

func (o *Options) findAndLoadPlugins() error {
	found := make(map[string]string)
	done := make(map[string][]string)

	for _, dir := range o.PluginDirs {
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				// don't fail when default plugin dir is missing
				if _, ok := err.(*os.PathError); ok && dir == DefaultPluginDir {
					return nil
				}

				log.Fatalf("failed to search for plugins: %s", err)
			}
			if info.IsDir() {
				return nil
			}
			if strings.HasSuffix(path, ".so") {
				name := filepath.Base(path)
				name = name[:len(name)-3] // strip suffix
				found[name] = path
				log.Printf("found plugin %s at %s", name, path)
			}
			return nil
		})
	}

	if err := o.loadPlugins(found, done); err != nil {
		return err
	}
	if err := o.loadFilterPlugins(found, done); err != nil {
		return err
	}
	if err := o.loadPredicatePlugins(found, done); err != nil {
		return err
	}
	if err := o.loadDataClientPlugins(found, done); err != nil {
		return err
	}

	for name, path := range found {
		log.Printf("attempting to load plugin from %s", path)
		mod, err := plugin.Open(path)
		if err != nil {
			return fmt.Errorf("open plugin %s from %s: %s", name, path, err)
		}

		conf, err := readPluginConfig(path)
		if err != nil {
			return fmt.Errorf("failed to read config for %s: %s", path, err)
		}

		if !pluginIsLoaded(done, name, "InitPlugin") {
			if sym, err := mod.Lookup("InitPlugin"); err == nil {
				fltrs, preds, dcs, err := initPlugin(sym, path, conf)
				if err != nil {
					return fmt.Errorf("filter plugin %s returned: %s", path, err)
				}
				o.CustomFilters = append(o.CustomFilters, fltrs...)
				o.CustomPredicates = append(o.CustomPredicates, preds...)
				o.CustomDataClients = append(o.CustomDataClients, dcs...)
				log.Printf("multitype plugin %s loaded from %s (filter: %d, predicate: %d, dataclient: %d)",
					name, path, len(fltrs), len(preds), len(dcs))
				markPluginLoaded(done, name, "InitPlugin")
			}
		} else {
			log.Printf("plugin %s already loaded with InitPlugin", name)
		}

		if !pluginIsLoaded(done, name, "InitFilter") {
			if sym, err := mod.Lookup("InitFilter"); err == nil {
				spec, err := initFilterPlugin(sym, path, conf)
				if err != nil {
					return fmt.Errorf("filter plugin %s returned: %s", path, err)
				}
				o.CustomFilters = append(o.CustomFilters, spec)
				log.Printf("filter plugin %s loaded from %s", name, path)
				markPluginLoaded(done, name, "InitFilter")
			}
		} else {
			log.Printf("plugin %s already loaded with InitFilter", name)
		}

		if !pluginIsLoaded(done, name, "InitPredicate") {
			if sym, err := mod.Lookup("InitPredicate"); err == nil {
				spec, err := initPredicatePlugin(sym, path, conf)
				if err != nil {
					return fmt.Errorf("predicate plugin %s returned: %s", path, err)
				}
				o.CustomPredicates = append(o.CustomPredicates, spec)
				log.Printf("predicate plugin %s loaded from %s", name, path)
				markPluginLoaded(done, name, "InitPredicate")
			}
		} else {
			log.Printf("plugin %s already loaded with InitPredicate", name)
		}

		if !pluginIsLoaded(done, name, "InitDataClient") {
			if sym, err := mod.Lookup("InitDataClient"); err == nil {
				spec, err := initDataClientPlugin(sym, path, conf)
				if err != nil {
					return fmt.Errorf("data client plugin %s returned: %s", path, err)
				}
				o.CustomDataClients = append(o.CustomDataClients, spec)
				log.Printf("data client plugin %s loaded from %s", name, path)
				markPluginLoaded(done, name, "InitDataClient")
			}
		} else {
			log.Printf("plugin %s already loaded with InitDataClient", name)
		}
	}

	var implementsMultiple []string
	for name, specs := range done {
		if len(specs) > 1 {
			implementsMultiple = append(implementsMultiple, name)
		}
	}
	if len(implementsMultiple) != 0 {
		return fmt.Errorf("found plugins implementing multiple Init* functions: %v", implementsMultiple)
	}
	return nil
}

func pluginIsLoaded(done map[string][]string, name, spec string) bool {
	loaded, ok := done[name]
	if !ok {
		return false
	}
	for _, s := range loaded {
		if s == spec {
			return true
		}
	}
	return false
}

func markPluginLoaded(done map[string][]string, name, spec string) {
	done[name] = append(done[name], spec)
}

func (o *Options) loadPlugins(found map[string]string, done map[string][]string) error {
	for _, plug := range o.Plugins {
		name := plug[0]
		path, ok := found[name]
		if !ok {
			return fmt.Errorf("multitype plugin %s not found in plugin dirs", name)
		}
		fltrs, preds, dcs, err := loadPlugin(path, plug[1:])
		if err != nil {
			return fmt.Errorf("failed to load plugin %s: %s", path, err)
		}

		o.CustomFilters = append(o.CustomFilters, fltrs...)
		o.CustomPredicates = append(o.CustomPredicates, preds...)
		o.CustomDataClients = append(o.CustomDataClients, dcs...)
		log.Printf("multitype plugin %s loaded from %s (filter: %d, predicate: %d, dataclient: %d)",
			name, path, len(fltrs), len(preds), len(dcs))
		markPluginLoaded(done, name, "InitPlugin")
	}
	return nil
}

func loadPlugin(path string, args []string) ([]filters.Spec, []routing.PredicateSpec, []routing.DataClient, error) {
	mod, err := plugin.Open(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open multitype plugin %s: %s", path, err)
	}

	conf, err := readPluginConfig(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read config for %s: %s", path, err)
	}

	sym, err := mod.Lookup("InitPlugin")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("lookup module symbol failed for %s: %s", path, err)
	}
	return initPlugin(sym, path, append(conf, args...))
}

func initPlugin(sym plugin.Symbol, path string, args []string) ([]filters.Spec, []routing.PredicateSpec, []routing.DataClient, error) {
	fn, ok := sym.(func([]string) ([]filters.Spec, []routing.PredicateSpec, []routing.DataClient, error))
	if !ok {
		return nil, nil, nil, fmt.Errorf("plugin %s's InitPlugin function has wrong signature", path)
	}
	fltrs, preds, dcs, err := fn(args)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("plugin %s returned: %s", path, err)
	}
	return fltrs, preds, dcs, nil
}

func (o *Options) loadFilterPlugins(found map[string]string, done map[string][]string) error {
	for _, fltr := range o.FilterPlugins {
		name := fltr[0]
		path, ok := found[name]
		if !ok {
			return fmt.Errorf("filter plugin %s not found in plugin dirs", name)
		}
		spec, err := loadFilterPlugin(path, fltr[1:])
		if err != nil {
			return fmt.Errorf("failed to load plugin %s: %s", path, err)
		}
		o.CustomFilters = append(o.CustomFilters, spec)
		log.Printf("loaded plugin %s (%s) from %s", name, spec.Name(), path)
		markPluginLoaded(done, name, "InitFilter")
	}
	return nil
}

func loadFilterPlugin(path string, args []string) (filters.Spec, error) {
	mod, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open filter plugin %s: %s", path, err)
	}

	conf, err := readPluginConfig(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config for %s: %s", path, err)
	}

	sym, err := mod.Lookup("InitFilter")
	if err != nil {
		return nil, fmt.Errorf("lookup module symbol failed for %s: %s", path, err)
	}
	return initFilterPlugin(sym, path, append(conf, args...))
}

func initFilterPlugin(sym plugin.Symbol, path string, args []string) (filters.Spec, error) {
	fn, ok := sym.(func([]string) (filters.Spec, error))
	if !ok {
		return nil, fmt.Errorf("plugin %s's InitFilter function has wrong signature", path)
	}
	spec, err := fn(args)
	if err != nil {
		return nil, fmt.Errorf("plugin %s returned: %s", path, err)
	}
	return spec, nil
}

func (o *Options) loadPredicatePlugins(found map[string]string, done map[string][]string) error {
	for _, pred := range o.PredicatePlugins {
		name := pred[0]
		path, ok := found[name]
		if !ok {
			return fmt.Errorf("predicate plugin %s not found in plugin dirs", name)
		}
		spec, err := loadPredicatePlugin(path, pred[1:])
		if err != nil {
			return fmt.Errorf("failed to load plugin %s: %s", path, err)
		}
		o.CustomPredicates = append(o.CustomPredicates, spec)
		log.Printf("loaded plugin %s (%s) from %s", name, spec.Name(), path)
		markPluginLoaded(done, name, "InitPredicate")
	}
	return nil
}

func loadPredicatePlugin(path string, args []string) (routing.PredicateSpec, error) {
	mod, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open predicate module %s: %s", path, err)
	}

	conf, err := readPluginConfig(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config for %s: %s", path, err)
	}
	sym, err := mod.Lookup("InitPredicate")
	if err != nil {
		return nil, fmt.Errorf("lookup module symbol failed for %s: %s", path, err)
	}
	return initPredicatePlugin(sym, path, append(conf, args...))
}

func initPredicatePlugin(sym plugin.Symbol, path string, args []string) (routing.PredicateSpec, error) {
	fn, ok := sym.(func([]string) (routing.PredicateSpec, error))
	if !ok {
		return nil, fmt.Errorf("plugin %s's InitPredicate function has wrong signature", path)
	}
	spec, err := fn(args)
	if err != nil {
		return nil, fmt.Errorf("plugin %s returned: %s", path, err)
	}
	return spec, nil
}

func (o *Options) loadDataClientPlugins(found map[string]string, done map[string][]string) error {
	for _, pred := range o.DataClientPlugins {
		name := pred[0]
		path, ok := found[name]
		if !ok {
			return fmt.Errorf("data client plugin %s not found in plugin dirs", name)
		}
		spec, err := loadDataClientPlugin(path, pred[1:])
		if err != nil {
			return fmt.Errorf("failed to load plugin %s: %s", path, err)
		}
		o.CustomDataClients = append(o.CustomDataClients, spec)
		log.Printf("loaded plugin %s from %s", name, path)
		markPluginLoaded(done, name, "InitDataClient")
	}
	return nil
}

func loadDataClientPlugin(path string, args []string) (routing.DataClient, error) {
	mod, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open data client module %s: %s", path, err)
	}

	conf, err := readPluginConfig(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config for %s: %s", path, err)
	}

	sym, err := mod.Lookup("InitDataClient")
	if err != nil {
		return nil, fmt.Errorf("lookup module symbol failed for %s: %s", path, err)
	}
	return initDataClientPlugin(sym, path, append(conf, args...))
}

func initDataClientPlugin(sym plugin.Symbol, path string, args []string) (routing.DataClient, error) {
	fn, ok := sym.(func([]string) (routing.DataClient, error))
	if !ok {
		return nil, fmt.Errorf("plugin %s's InitDataClient function has wrong signature", path)
	}
	spec, err := fn(args)
	if err != nil {
		return nil, fmt.Errorf("module %s returned: %s", path, err)
	}
	return spec, nil
}

func readPluginConfig(plugin string) (conf []string, err error) {
	data, err := os.ReadFile(plugin[:len(plugin)-3] + ".conf")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line[0] != '#' {
			conf = append(conf, line)
		}
	}
	return conf, nil
}
