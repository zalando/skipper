package helpers

import "regexp"

// KVRegexPair
type KVRegexPair struct {
	Key   string
	Regex *regexp.Regexp
}

// NewKVRegexPair creates a new KVRegexPair
func NewKVRegexPair(key string, regex *regexp.Regexp) KVRegexPair {
	return KVRegexPair{Key: key, Regex: regex}
}

func KVRegexPairToArgs(pairs []KVRegexPair) []interface{} {
	args := make([]interface{}, 0, len(pairs)*2)
	for _, pair := range pairs {
		args = append(args, pair.Key, pair.Regex.String())
	}
	return args
}

// KVPair
type KVPair struct {
	Key, Value string
}

// NewKVPair creates a new KVPair
func NewKVPair(key, value string) KVPair {
	return KVPair{Key: key, Value: value}
}

func KVPairToArgs(pairs []KVPair) []interface{} {
	args := make([]interface{}, 0, len(pairs)*2)
	for _, pair := range pairs {
		args = append(args, pair.Key, pair.Value)
	}
	return args
}
