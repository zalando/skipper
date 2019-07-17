package main

import (
	"strings"
)

const (
	filterPluginUsage     = "set a custom filter plugins to load, a comma separated list of name and arguments"
	predicatePluginUsage  = "set a custom predicate plugins to load, a comma separated list of name and arguments"
	dataclientPluginUsage = "set a custom dataclient plugins to load, a comma separated list of name and arguments"
	multiPluginUsage      = "set a custom multitype plugins to load, a comma separated list of name and arguments"
)

type pluginFlag struct {
	listFlag *listFlag
	values   [][]string
}

func newPluginFlag() *pluginFlag {
	return &pluginFlag{listFlag: newListFlag(" ")}
}

func (f pluginFlag) String() string {
	return f.listFlag.String()
}

func (f *pluginFlag) Set(value string) error {
	if err := f.listFlag.Set(value); err != nil {
		return err
	}

	for _, v := range f.listFlag.values {
		f.values = append(f.values, strings.Split(v, ","))
	}

	return nil
}
