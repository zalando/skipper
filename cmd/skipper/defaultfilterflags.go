package main

import (
	"fmt"

	"github.com/zalando/skipper/eskip"
)

const (
	defaultPrependFiltersUsage = "set of default filters to apply to prepend to all filters of all routes"
	defaultAppendFiltersUsage  = "set of default filters to apply to append to all filters of all routes"
)

type defaultFiltersFlags struct {
	filters []*eskip.Filter
}

func (dpf *defaultFiltersFlags) String() string {
	return "<default filters>"
}

func (dpf *defaultFiltersFlags) Set(value string) error {
	if dpf == nil {
		dpf = &defaultFiltersFlags{}
	}

	fs, err := eskip.ParseFilters(value)
	if err != nil {
		return fmt.Errorf("failed to parse default filters: %v", err)
	}

	dpf.filters = append(dpf.filters, fs...)
	return nil
}

func (dpf *defaultFiltersFlags) Get() []*eskip.Filter {
	return dpf.filters
}
