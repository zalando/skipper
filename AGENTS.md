# AGENTS.md

## Project overview

Skipper is a http proxy library written in Go. The project layout is library first and has some main packages in sub-diretories of the ./cmd folder.
Eskip is the syntax used by routing specifications.
Dataclients fetch routing information and create a list of eskip.Routes.
The routing package fetch eskip.Routes by the dataclients, run routing.PreProcessors on all eskip.Routes, converts eskip.Routes to routing.Routes, run routing.PostProcessors on routing.Routes and replace the current routing tree.
Routes consists of predicates, filters and backend.
Predicates match requests to select the best matching route.
Filters can process the http.Request and the http.Response.
Backend is the thing skipper should proxy to for example:
- network: URL to proxy to
- shunt: respond from proxy to client
- loopback: evaluate again the routing tree
- load balancer: select lb algorithm and list all endpoints with scheme, IP (or hostname) and port

## Setup commands

- Install deps: `make deps`
- Build project: `make`
- Start example proxy with one route: `./bin/skipper -inline-routes='r: * -> latency("1ms") -> status(201) -> <shunt>' -address :9001`
- Run tests by package for example proxy: `go test ./proxy`
- Run all tests: `make check`
- Run all tests with race detector: `make check-race`

## Code style

- Use idiomatic Go
- go doc on exposed things
- package docs in doc.go
- no comments in code if they are not critical
- no kubernetes client-go dependencies

## Testing instructions

- Use table driven tests if it makes sense
- Find test helpers in subpackages like net/nettest
- We use github.com/AlexanderYastrebov/noleak to enforce no leaks like goroutine leak or channel leak or not closed http.Body for example

## PR instructions

- Human has to write PR description
- If you create a new filter, predicate or dataclient a PR is fine, else a human has to write an issue.
- Always run `make fmt`, `make lint` and `make shortcheck` before committing.

## Security

- Every new package added in go.mod has to be reviewed by a human
