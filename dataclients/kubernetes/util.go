package kubernetes

import "sort"

// looking forward for generic types
func getSortedKeysStr(h map[string]string) []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func stringToEmptyInterface(a []string) []interface{} {
	res := make([]interface{}, len(a))
	for i := range a {
		res[i] = a[i]
	}
	return res
}
