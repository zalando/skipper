package kubernetes

import (
	"fmt"
	"strings"
	// "github.com/zalando/skipper/eskip"
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

/*
// currently only used for RG
func hostCatchAllRoutes(hostRoutes map[string][]*eskip.Route) []*eskip.Route {
	// take canonical to make sure that every predicate is in the predicates list
	return nil
}
*/
