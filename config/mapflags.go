package config

import (
	"fmt"
	"strings"
)

// mapFlags are generic string key-value pair flags.
// Use when option keys are not predetermined.
type mapFlags struct {
	values map[string]string
}

const formatErrorString = "invalid map key-value pair, expected format key=value but got: '%v'"

func newMapFlags() *mapFlags {
	return &mapFlags{
		values: make(map[string]string),
	}
}

func (m *mapFlags) String() string {
	var pairs []string

	for k, v := range m.values {
		pairs = append(pairs, fmt.Sprint(k, "=", v))
	}

	return strings.Join(pairs, "'")
}

func (m *mapFlags) Set(value string) error {
	if m == nil {
		return nil
	}

	m.values = make(map[string]string)

	vs := strings.Split(value, ",")
	for _, vi := range vs {
		kv := strings.SplitN(vi, "=", 2)

		if len(kv) != 2 {
			return fmt.Errorf(formatErrorString, vi)
		}

		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])

		if k == "" || v == "" {
			return fmt.Errorf(formatErrorString, vi)
		}

		m.values[k] = v
	}

	return nil
}

func (m *mapFlags) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var values = make(map[string]string)
	if err := unmarshal(&values); err != nil {
		return err
	}

	m.values = values

	return nil
}
