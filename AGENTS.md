# AGENTS.md

## Project overview

Skipper is a http proxy library written in Go. The project layout is library first and has some main packages in sub-directories of the ./cmd folder.
Eskip is the syntax used by routing specifications.
Dataclients fetch routing information and create a list of eskip.Routes.
The routing package fetch eskip.Routes by the dataclients, run routing.PreProcessors on all eskip.Routes, converts eskip.Routes to routing.Routes, run routing.PostProcessors on routing.Routes and replace the current routing tree.
Routes consists of predicates, filters and backend.
Predicates match requests to select the best matching route.
Filters can process the http.Request and the http.Response.
Backend is the thing to which skipper should proxy, for example:
- network: URL to proxy to
- shunt: respond from proxy to client
- loopback: evaluate again the routing tree
- load balancer: select lb algorithm and list all endpoints with scheme, IP (or hostname) and port

## Setup commands

- Install deps: `make deps`
- Build project: `make`
- Start example proxy with one route: `./bin/skipper -inline-routes='r: * -> latency("1ms") -> status(201) -> <shunt>' -address :9001`
  Call the proxy: curl http://localhost:9001/
- Run tests by package for example proxy: `go test ./proxy`
- Run linter: `make lint`
- Run all tests: `make check`
- Run all tests with race detector: `make check-race`

## Code style

- Use idiomatic Go
- Go doc on exposed things
- Package docs in doc.go
- No comments in code if they are not critical
- No kubernetes client-go dependencies
- Run linter `make lint` and fix all findings

## Communication Style

- Use domain-specific nouns and verbs in all communication. The vocabulary is maintained in `CONTEXT.md` and must be updated continuously as new domain concepts are identified.
- Be concise. Challenge ideas with rationale and citations when appropriate.

## Updating CONTEXT.md

When a new domain term is identified (e.g., a new filter category, routing concept, or infrastructure component), add it to `CONTEXT.md` following this format:

```md
**Term**:
One or two sentence definition of what it IS. No implementation details.
_Avoid_: synonym1, synonym2
```

Rules:
- Be opinionated — pick the best word, list others under `_Avoid_`.
- Keep definitions tight (1-2 sentences). Define what it IS, not how it works internally.
- No interface signatures, struct fields, function names, or step-by-step internals.
- Only include terms specific to Skipper's domain. General programming concepts don't belong.
- `_Avoid_` is optional — only add it when a synonym trap exists that would confuse communication.

## Testing instructions

- Use table driven tests if it makes sense
- Find test helpers in subpackages like net/nettest
- We use github.com/AlexanderYastrebov/noleak to enforce no leaks like goroutine leak or channel leak or not closed http.Body for example
- Test also error cases

## PR instructions

- Human has to write PR description
- If you create a new filter, predicate or dataclient a PR is fine, else a human has to write an issue.
- Always run `make fmt`, `make lint` and `make shortcheck` before committing.
- Use `git commit --signoff` to comply with [DCO](https://developercertificate.org/).

## Security

- Every new package added in go.mod has to be reviewed by a human
