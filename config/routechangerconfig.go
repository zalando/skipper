package config

import (
	"fmt"
	"regexp"
	"strings"
)

type routeChangerConfig struct {
	Reg  *regexp.Regexp
	Repl string
	Sep  string
}

func (rcc routeChangerConfig) String() string {
	if rcc.Reg == nil {
		return ""
	}
	return fmt.Sprintf("%s%s%s%s%s", rcc.Sep, rcc.Reg, rcc.Sep, rcc.Repl, rcc.Sep)
}

func (rcc *routeChangerConfig) Set(value string) error {
	if len(value) == 0 {
		return fmt.Errorf("empty string as an argument is not allowed")
	}
	firstSym := value[:1]
	a := strings.Split(value, firstSym)
	if len(a) != 4 {
		return fmt.Errorf("unexpected size of string split: %d", len(a))
	}
	var err error
	reg, repl := a[1], a[2]
	rcc.Reg, err = regexp.Compile(reg)
	rcc.Repl = repl
	rcc.Sep = firstSym
	return err
}

func (rcc *routeChangerConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	return rcc.Set(value)
}
