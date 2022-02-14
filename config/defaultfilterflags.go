package config

import (
	"fmt"

	"github.com/zalando/skipper/eskip"
)

type defaultFiltersFlags struct {
	filters []*eskip.Filter
}

func (dpf defaultFiltersFlags) String() string {
	return "<default filters>"
}

func (dpf *defaultFiltersFlags) Set(value string) error {
	fs, err := eskip.ParseFilters(value)
	if err != nil {
		return fmt.Errorf("failed to parse default filters: %v", err)
	}

	dpf.filters = fs
	return nil
}

func (dpf *defaultFiltersFlags) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	return dpf.Set(value)
}
