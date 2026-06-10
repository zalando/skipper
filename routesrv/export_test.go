package routesrv

import (
	"context"
	"net/http"
	"time"
)

func SetNow(rs *RouteServer, now func() time.Time) {
	rs.poller.b.now = now
}

func (rs *RouteServer) ListenAndServe() (err error) {
	if tlsConfig := rs.server.TLSConfig; tlsConfig != nil {
		if err = rs.server.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
			rs.server.Shutdown(context.Background())
		} else {
			err = nil
		}
	} else {
		if err = rs.server.ListenAndServe(); err != http.ErrServerClosed {
			rs.server.Shutdown(context.Background())
		} else {
			err = nil
		}
	}

	rs.wg.Wait()
	return
}
