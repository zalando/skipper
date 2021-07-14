package config

import (
	"regexp"
	"strings"
)

type regexpListFlag []*regexp.Regexp

func (r regexpListFlag) String() string {
	s := make([]string, len(r))
	for i, ri := range r {
		s[i] = ri.String()
	}

	return strings.Join(s, "\n")
}

func (r *regexpListFlag) Set(value string) error {
	rx, err := regexp.Compile(value)
	if err != nil {
		return err
	}

	*r = append(*r, rx)
	return nil
}

func (r *regexpListFlag) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	rx, err := regexp.Compile(s)
	if err != nil {
		return err
	}

	*r = append(*r, rx)
	return nil
}
