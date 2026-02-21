package config

import (
	"fmt"
	"regexp"
	"strings"
)

type routeChangerConfigItem struct {
	Reg  *regexp.Regexp `yaml:"reg"`
	Repl string         `yaml:"repl"`
	Sep  string         `yaml:"sep"`
}

func (rcci routeChangerConfigItem) String() string {
	return rcci.Sep + rcci.Reg.String() + rcci.Sep + rcci.Repl + rcci.Sep
}

type routeChangerConfig []routeChangerConfigItem

func (rcc routeChangerConfig) String() string {
	var b strings.Builder
	for i, rcci := range rcc {
		if i > 0 {
			b.WriteString("\n")
		}

		b.WriteString(rcci.String())
	}
	return b.String()
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

	regex, err := regexp.Compile(reg)
	if err != nil {
		return err
	}

	rcci := routeChangerConfigItem{
		Reg:  regex,
		Repl: repl,
		Sep:  firstSym,
	}
	*rcc = append(*rcc, rcci)
	return err
}

func (rcc *routeChangerConfig) UnmarshalYAML(unmarshal func(any) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	return rcc.Set(value)
}
