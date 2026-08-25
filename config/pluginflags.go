package config

import (
	"strings"
)

type pluginFlag struct {
	listFlag *listFlag
	values   [][]string
}

func newPluginFlag() *pluginFlag {
	return &pluginFlag{listFlag: newListFlag(" ")}
}

func (f pluginFlag) String() string {
	if f.listFlag == nil {
		return ""
	}

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

func (f *pluginFlag) UnmarshalYAML(unmarshal func(any) error) error {
	var value map[string][]string
	if err := unmarshal(&value); err != nil {
		return err
	}

	for k, values := range value {
		plugin := append([]string{k}, values...)
		f.values = append(f.values, plugin)
	}

	return nil
}
