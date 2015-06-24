// This package contains middleware for filters.
//
// To create a filtering middleware, create first a subdirectory, conventionally with the name of your
// middleware, implement the skipper.Middleware and skipper.Filter interfaces, and add the registering call
// to the Register function in middleware.go.
//
// For convenience, the noop middleware can be composed into the implemented middleware, and only the so the
// implementation can shadow only the methods that are relevant ("override").
package middleware
