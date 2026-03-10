package proxy

import (
	"github.com/zalando/skipper/routing"
)

type zoneAwareEndpoints struct {
	zone             string
	endpointRegistry *routing.EndpointRegistry
}

func (zae *zoneAwareEndpoints) filterZoneEndpoints(ctx *context, endpoints []routing.LBEndpoint) []routing.LBEndpoint {
	if zae == nil {
		return endpoints
	}

	ctx.logger.Debugf("foo")
	zae.endpointRegistry.GetEndpoints("foo")
	return nil
}
