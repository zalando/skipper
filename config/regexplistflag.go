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

func (r *regexpListFlag) UnmarshalYAML(unmarshal func(any) error) error {
	var m map[string][]string
	if err := unmarshal(&m); err != nil {
		return err
	}

	for _, value := range m {
		for _, item := range value {
			rx, err := regexp.Compile(item)
			if err != nil {
				return err
			}

			*r = append(*r, rx)
		}
	}

	return nil
}
