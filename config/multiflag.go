package config

import (
	"strings"
)

type multiFlag []string

func (i *multiFlag) String() string {
	return strings.Join(*i, " ")
}

func (i *multiFlag) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func (r *multiFlag) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var values []string
	if err := unmarshal(values); err != nil {
		return err
	}
	*r = values
	return nil
}
