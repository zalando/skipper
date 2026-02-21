package config

import (
	"strings"
)

type multiFlag []string

func (f *multiFlag) String() string {
	return strings.Join(*f, " ")
}

func (f *multiFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func (f *multiFlag) UnmarshalYAML(unmarshal func(any) error) error {
	var values []string
	if err := unmarshal(&values); err != nil {
		return err
	}
	*f = values
	return nil
}
