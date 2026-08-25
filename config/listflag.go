package config

import (
	"fmt"
	"strings"
)

type listFlag struct {
	sep     string
	allowed map[string]bool
	value   string
	values  []string
}

func newListFlag(sep string, allowed ...string) *listFlag {
	lf := &listFlag{
		sep:     sep,
		allowed: make(map[string]bool),
	}

	for _, a := range allowed {
		lf.allowed[a] = true
	}

	return lf
}

func commaListFlag(allowed ...string) *listFlag {
	return newListFlag(",", allowed...)
}

func (lf *listFlag) Set(value string) error {
	if lf == nil {
		return nil
	}

	if value == "" {
		lf.value = ""
		lf.values = nil
	} else {
		lf.value = value
		lf.values = strings.Split(value, lf.sep)
	}

	if err := lf.validate(); err != nil {
		return err
	}

	return nil
}

func (lf *listFlag) UnmarshalYAML(unmarshal func(any) error) error {
	var values []string
	if err := unmarshal(&values); err != nil {
		return err
	}

	lf.value = strings.Join(values, lf.sep)
	lf.values = values

	if err := lf.validate(); err != nil {
		return err
	}

	return nil
}

func (lf *listFlag) validate() error {
	if len(lf.allowed) == 0 {
		return nil
	}

	for _, v := range lf.values {
		if !lf.allowed[v] {
			return fmt.Errorf("value not allowed: %s", v)
		}
	}
	return nil
}

func (lf listFlag) String() string { return lf.value }
