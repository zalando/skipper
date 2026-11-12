// Package hedge implements adaptive hedging for load-balanced routes. The hedge
// filter races response headers for eligible bodyless GET and HEAD requests and
// shares a per-route additional-attempt allowance with dial-failure retries.
//
// Skipper's standard setup registers the filter and its route-state postprocessor
// for normal and priority routes. Library integrations that construct routing
// options directly must register the returned specification as both a filter
// specification and a routing postprocessor. Each independently updated route
// population, including priority routes, needs its own postprocessor registry.
package hedge
