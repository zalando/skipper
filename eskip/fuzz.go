//go:build gofuzz
// +build gofuzz

package eskip

func FuzzParse(data []byte) int {
	_, _ = Parse(string(data))
	return 0
}
