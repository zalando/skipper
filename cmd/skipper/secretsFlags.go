package main

import (
	"strings"
)

const credentialPathsUsage = "directories or files to watch for credentials to use by bearerinjector filter"

type secretsFlags struct {
	values []string
}

func (sf *secretsFlags) String() string {
	return strings.Join(sf.values, ",")
}

func (sf *secretsFlags) Set(value string) error {
	if sf == nil {
		sf = &secretsFlags{}
	}
	sf.values = strings.Split(value, ",")

	return nil
}

func (sf *secretsFlags) Get() []string {
	return sf.values
}
