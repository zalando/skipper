package main

import (
	"fmt"
	"strings"
)

type listFlag struct {
	sep     string
	allowed map[string]bool
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

	lf.values = strings.Split(value, lf.sep)
	if len(lf.allowed) == 0 {
		return nil
	}

	for _, v := range lf.values {
		if !lf.allowed[v] {
			return fmt.Errorf("flag value not allowed: %s", v)
		}
	}

	return nil
}

func (lf listFlag) String() string { return strings.Join(lf.values, lf.sep) }
