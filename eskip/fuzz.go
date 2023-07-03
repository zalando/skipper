//go:build fuzz
// +build fuzz

package eskip

func FuzzParse(data []byte) int {
	_, _ = Parse(string(data))
	return 0
}
