package main

import (
	"errors"
	"strings"
)

const metricsFlavourUsage = "Metrics flavour is used to change the exposed metrics format. Supported metric formats: 'codahale' and 'prometheus', you can select both of them"

type metricsFlags struct {
	values []string
}

var (
	errInvalidMetricsFlag = errors.New("invalid metrics flavour, valid ones are 'codahale' and 'prometheus'")
	allowed               = map[string]bool{
		"codahale":   true,
		"prometheus": true,
	}
)

func (m *metricsFlags) String() string {
	return strings.Join(m.values, ",")
}

func (m *metricsFlags) Set(value string) error {
	if m == nil {
		m = &metricsFlags{}
	}
	m.values = strings.Split(value, ",")

	for _, s := range m.values {
		if _, ok := allowed[s]; !ok {
			return errInvalidMetricsFlag
		}
	}
	return nil
}

func (m *metricsFlags) Get() []string {
	return m.values
}
