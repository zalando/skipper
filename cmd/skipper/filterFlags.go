package main

import (
	"strings"
)

const filterPluginUsage = "set a custom filter plugins to load, a comma separated list of name and arguments"

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
	for _, v := range strings.Split(value, " ") {
		f.values = append(f.values, strings.Split(v, ","))
	}
	return nil
}

func (f *filterFlags) Get() [][]string {
	return f.values
}

const predicatePluginUsage = "set a custom predicate plugins to load, a comma separated list of name and arguments"

type predicateFlags struct {
	values [][]string
}

func (f *predicateFlags) String() string {
	var ret []string
	for _, val := range f.values {
		ret = append(ret, strings.Join(val, ","))
	}
	return strings.Join(ret, " ")
}

func (f *predicateFlags) Set(value string) error {
	if f == nil {
		f = &predicateFlags{}
	}
	for _, v := range strings.Split(value, " ") {
		f.values = append(f.values, strings.Split(v, ","))
	}
	return nil
}

func (f *predicateFlags) Get() [][]string {
	return f.values
}

const dataclientPluginUsage = "set a custom dataclient plugins to load, a comma separated list of name and arguments"

type dataclientFlags struct {
	values [][]string
}

func (f *dataclientFlags) String() string {
	var ret []string
	for _, val := range f.values {
		ret = append(ret, strings.Join(val, ","))
	}
	return strings.Join(ret, " ")
}

func (f *dataclientFlags) Set(value string) error {
	if f == nil {
		f = &dataclientFlags{}
	}
	for _, v := range strings.Split(value, " ") {
		f.values = append(f.values, strings.Split(v, ","))
	}
	return nil
}

func (f *dataclientFlags) Get() [][]string {
	return f.values
}
