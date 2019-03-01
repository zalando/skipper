package main

import (
	"strings"
)

const swarmRedisURLsUsage = "Redis URLs as comma separated list, used for building a swarm, for example in redis based cluster ratelimits"

type swarmRedisFlags struct {
	redisURLs []string
}

func (sf *swarmRedisFlags) String() string {
	return strings.Join(sf.redisURLs, ",")
}

func (sf *swarmRedisFlags) Set(value string) error {
	if sf == nil {
		sf = &swarmRedisFlags{}
	}
	sf.redisURLs = strings.Split(value, ",")

	return nil
}

func (sf *swarmRedisFlags) Get() []string {
	return sf.redisURLs
}
