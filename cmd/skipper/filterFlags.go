package main

import (
	"strings"
)

const filterFlagsUsage = "none yet"

type filterFlags struct {
	values [][]string
}

func (f *filterFlags) String() string {
	var ret []string
	for _, val := range f.values {
		ret = append(ret, strings.Join(val, ","))
	}
	return strings.Join(ret, " ")
}

func (f *filterFlags) Set(value string) error {
	if f == nil {
		f = &filterFlags{}
	}
	f.values = nil
	for _, v := range strings.Split(value, " ") {
		f.values = append(f.values, strings.Split(v, ","))
	}
	return nil
}

func (f *filterFlags) Get() [][]string {
	return f.values
}
