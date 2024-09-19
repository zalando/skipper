package auth

import (
	"fmt"

	"github.com/ghodss/yaml"
)

// yamlConfigParser parses and caches yaml configurations of type T.
// Use [newYamlConfigParser] to create instances and ensure that *T implements [yamlConfig].
type yamlConfigParser[T any] struct {
	initialize func(*T) error
	cacheSize  int
	cache      map[string]*T
}

// yamlConfig must be implemented by config value pointer type.
// It is used to initialize the value after parsing.
type yamlConfig interface {
	initialize() error
}

// newYamlConfigParser creates a new parser with a given cache size.
func newYamlConfigParser[T any, PT interface {
	*T
	yamlConfig
}](cacheSize int) yamlConfigParser[T] {
	// We want user to specify config type T but ensure that *T implements [yamlConfig].
	//
	// Type inference only works for functions but not for types
	// (see https://github.com/golang/go/issues/57270 and https://github.com/golang/go/issues/51527)
	// therefore we create instances using function with two type parameters
	// but second parameter is inferred from the first so the caller does not have to specify it.
	//
	// To use *T.initialize we setup initialize field
	return yamlConfigParser[T]{
		initialize: func(v *T) error { return PT(v).initialize() },
		cacheSize:  cacheSize,
		cache:      make(map[string]*T, cacheSize),
	}
}

// parseSingleArg calls [yamlConfigParser.parse] with the first string argument.
// If args slice does not contain a single string, it returns an error.
func (p *yamlConfigParser[T]) parseSingleArg(args []any) (*T, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("requires single string argument")
	}
	config, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("requires single string argument")
	}
	return p.parse(config)
}

// parse parses a yaml configuration or returns a cached value
// if the exact configuration was already parsed before.
// Returned value is shared by multiple callers and therefore must not be modified.
func (p *yamlConfigParser[T]) parse(config string) (*T, error) {
	if v, ok := p.cache[config]; ok {
		return v, nil
	}

	v := new(T)
	if err := yaml.Unmarshal([]byte(config), v); err != nil {
		return nil, err
	}

	if err := p.initialize(v); err != nil {
		return nil, err
	}

	// evict random element if cache is full
	if p.cacheSize > 0 && len(p.cache) == p.cacheSize {
		for k := range p.cache {
			delete(p.cache, k)
			break
		}
	}

	p.cache[config] = v

	return v, nil
}
