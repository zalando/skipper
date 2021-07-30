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
	reg, repl := a[1], a[2]
	rcc.Reg = regexp.MustCompile(reg)
	rcc.Repl = []byte(repl)
	return nil
}

func (rcc *routeChangerConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	return rcc.Set(value)
}
