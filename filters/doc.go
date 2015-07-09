// This package contains specifications for filters.
//
// To create a filterin, create first a subdirectory, conventionally with the name of your
// filter, implement the skipper.FilterSpec and skipper.Filter interfaces, and add the registering call
// to the Register function in filters.go.
//
// For convenience, the noop filter can be composed into the implemented filter, and only the so the
// implementation can shadow only the methods that are relevant ("override").
package filters
