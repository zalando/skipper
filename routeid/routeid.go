package routeid
import (
"regexp"
"github.com/zalando/skipper/filters/flowid"
)

const randomIdLength = 16

var routeIdRx = regexp.MustCompile("\\W")

// generate weak random id for a route if
// it doesn't have one.
func GenerateIfNeeded(existingId string) (string) {
	if existingId != "" {
		return existingId
	}

	// using this to avoid adding a new dependency.
	id, err := flowid.NewFlowId(randomIdLength)
	if err != nil {
		return existingId
	}

	// replace characters that are not allowed
	// for eskip route ids.
	id = routeIdRx.ReplaceAllString(id, "x")
	return "route" + id
}
