package kubernetes

import (
	"fmt"
	"strings"
)

func rxDots(h string) string {
	return strings.Replace(h, ".", "[.]", -1)
}

func createHostRx(h ...string) string {
	if len(h) == 0 {
		return ""
	}

	hrx := make([]string, len(h))
	for i := range h {
		hrx[i] = rxDots(h[i])
	}

	return fmt.Sprintf("^(%s)$", strings.Join(hrx, "|"))
}
