package config

import (
	"fmt"
	"strings"

	"github.com/zalando/skipper/eskip"
)

type defaultFiltersFlags struct {
	values  []string
	filters []*eskip.Filter
}

func (dpf defaultFiltersFlags) String() string {
	return strings.Join(dpf.values, " -> ")
}

func (dpf *defaultFiltersFlags) Set(value string) error {
	fs, err := eskip.ParseFilters(value)
	if err != nil {
		return fmt.Errorf("failed to parse default filters: %w", err)
	}
	if len(fs) > 0 {
		dpf.values = append(dpf.values, value)
		dpf.filters = append(dpf.filters, fs...)
	}
	return nil
}

func (dpf *defaultFiltersFlags) UnmarshalYAML(unmarshal func(any) error) error {
	values := make([]string, 1)
	if err := unmarshal(&values); err != nil {
		// Try to unmarshal as string for backwards compatibility.
		// UnmarshalYAML allows calling unmarshal more than once.
		if err := unmarshal(&values[0]); err != nil {
			return err
		}
	}
	dpf.values = nil
	dpf.filters = nil
	for _, v := range values {
		if err := dpf.Set(v); err != nil {
			return err
		}
	}
	return nil
}
