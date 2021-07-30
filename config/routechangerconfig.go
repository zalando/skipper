package config

import (
	"fmt"
	"regexp"
	"strings"
)

type routeChangerConfig struct {
	Reg  *regexp.Regexp
	Repl []byte
}

func (rcc routeChangerConfig) String() string {
	if rcc.Reg == nil {
		return ""
	}
	return fmt.Sprintf("/%s/%s/", rcc.Reg, rcc.Repl)
}

func (rcc *routeChangerConfig) Set(value string) error {
	a := strings.Split(value, "/")
	if len(a) != 4 {
		return fmt.Errorf("unexpected size of string split: %d", len(a))
	}
	var err error
	reg, repl := a[1], a[2]
	rcc.Reg, err = regexp.Compile(reg)
	rcc.Repl = []byte(repl)
	return err
}

func (rcc *routeChangerConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	return rcc.Set(value)
}
