package middleware

import (
	"skipper/middleware/requestheader"
	"skipper/middleware/xalando"
	"skipper/skipper"
)

func RegisterMiddleware(registry skipper.MiddlewareRegistry) {
	registry.Add(
		requestheader.Make(),
		xalando.Make(),
	)
}
