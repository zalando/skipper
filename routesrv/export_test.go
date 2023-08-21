package routesrv

import "time"

func SetNow(rs *RouteServer, now func() time.Time) {
	rs.poller.b.now = now
}
