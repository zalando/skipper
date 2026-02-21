package config

import (
	"fmt"

	"gopkg.in/yaml.v2"
)

type yamlFlag[T any] struct {
	Ptr   **T
	value string // only for Set
}

func newYamlFlag[T any](ptr **T) *yamlFlag[T] {
	return &yamlFlag[T]{Ptr: ptr}
}

func (yf *yamlFlag[T]) Set(value string) error {
	var opts T
	if err := yaml.Unmarshal([]byte(value), &opts); err != nil {
		return fmt.Errorf("failed to parse yaml: %w", err)
	}
	*yf.Ptr = &opts
	yf.value = value
	return nil
}

func (yf *yamlFlag[T]) UnmarshalYAML(unmarshal func(any) error) error {
	var opts T
	if err := unmarshal(&opts); err != nil {
		return err
	}
	*yf.Ptr = &opts
	return nil
}

func (yf *yamlFlag[T]) String() string {
	if yf == nil {
		return ""
	}
	return yf.value
}
