package routing

import "time"

var (
	ExportProcessRouteDef   = processRouteDef
	ExportNewMatcher        = newMatcher
	ExportMatch             = (*matcher).match
	ExportProcessPredicates = processPredicates
	ExportLastSeenTimeout   = lastSeenTimeout
)

func SetNow(r *EndpointRegistry, now func() time.Time) {
	r.now = now
}
