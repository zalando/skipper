package main

import (
	"strings"
)

const (
	filterPluginUsage     = "set a custom filter plugins to load, a comma separated list of name and arguments"
	predicatePluginUsage  = "set a custom predicate plugins to load, a comma separated list of name and arguments"
	dataclientPluginUsage = "set a custom dataclient plugins to load, a comma separated list of name and arguments"
)

type pluginFlags struct {
	values [][]string
}

func (f *pluginFlags) String() string {
	var ret []string
	for _, val := range f.values {
		ret = append(ret, strings.Join(val, ","))
	}
	return strings.Join(ret, " ")
}

func (f *pluginFlags) Set(value string) error {
	for _, v := range strings.Split(value, " ") {
		f.values = append(f.values, strings.Split(v, ","))
	}
	return nil
}

func (f *pluginFlags) Get() [][]string {
	return f.values
}

const multiPluginUsage = "set a custom multitype plugins to load, a comma separated list of name and arguments"

type multiPluginFlags struct {
	values [][]string
}

func (f *multiPluginFlags) String() string {
	var ret []string
	for _, val := range f.values {
		ret = append(ret, strings.Join(val, ","))
	}
	return strings.Join(ret, " ")
}

func (f *multiPluginFlags) Set(value string) error {
	if f == nil {
		f = &multiPluginFlags{}
	}
	for _, v := range strings.Split(value, " ") {
		f.values = append(f.values, strings.Split(v, ","))
	}
	return nil
}

func (f *multiPluginFlags) Get() [][]string {
	return f.values
}
