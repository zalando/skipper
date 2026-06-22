# Skipper

Ubiquitous language for the Skipper. Use these terms consistently in all communication.

## Core Concepts

**Eskip**:
The domain-specific language used to define routing rules (predicates, filters, backend).

**Route**:
A routing rule that matches incoming HTTP requests and defines how they are processed and where they are forwarded.

**Route Definition**:
The static eskip representation of a route (predicates, filters, backend string).
_Avoid_: route config, route spec

**Route Instance**:
The compiled runtime representation of a route with instantiated filters and predicates, ready for matching.

**Route ID**:
A unique identifier for a route within a routing table (e.g., `"kube__healthz_down"`, `"kube__redirect"`).

**Routing Tree**:
An immutable radix tree built from route instances, used for fast path-based route lookup.

**Route Matching**:
The process of finding the best route for a request: path tree lookup (only `Path()` and `PathSubtree()` participate), then predicate evaluation (including `PathRegexp`), then weight resolution among multiple matches. If no full match at the most specific path, backtracking tries less specific path nodes.

**Atomic Switching**:
Replacing the active routing tree with a newly built one without blocking in-flight requests.
_Avoid_: hot reload, live update

## Backends

**Backend**:
The target destination to which a matched request is forwarded, or the proxy-internal handler that generates the response (shunt, loopback).

**Network Backend**:
Proxies the request to a single URL (e.g., `"https://api.example.com"`).

**Shunt Backend**:
Responds directly from the proxy without forwarding. Eskip syntax: `<shunt>`.
_Avoid_: internal handler

**Loopback Backend**:
Re-evaluates the request against the routing tree after filter modifications. Eskip syntax: `<loopback>`.

**Dynamic Backend**:
Backend URL determined at runtime by filters (e.g., `setDynamicBackendUrl`). Eskip syntax: `<dynamic>`.

**LB Backend**:
Distributes requests across multiple endpoints using a load-balancing algorithm. Eskip syntax: `<algorithm, "http://host1:port", "http://host2:port">`. Endpoints must be full URLs.
_Avoid_: load balanced target

**Forward Backend**:
Rewritten by the `ForwardPreProcessor` to a configured target, then collapsed into a Network Backend.

## Predicates

**Predicate**:
A matching condition that determines whether a route applies to a given request. Multiple predicates on a route are AND-ed.

**PredicateSpec**:
Factory interface that creates predicate instances from eskip arguments. Defines `Name()` and `Create()`.

### Key Named Predicates

**Path**:
Matches an exact URL path with optional named (`:param`) and free (`*param`) wildcards. Used in the radix tree for O(log n) lookup.

**PathSubtree**:
Matches a path prefix and all sub-paths beneath it. Also tree-indexed.

**PathRegexp**:
Matches the URL path against a regular expression. Evaluated after tree lookup; O(n) over all routes using it.

**Host**:
Matches the request `Host` header against a regular expression.

**HostAny**:
Matches the request `Host` header by literal exact equality against a list of hostnames. Distinct from regexp-based `Host`.

**Method**:
Matches a single HTTP method (e.g., `GET`, `POST`).

**Header**:
Matches an exact request header name/value pair.

Full catalog: `docs/reference/predicates.md`

## Filters

**Filter**:
A processing unit that modifies the request (before backend) or response (after backend) in a pipeline.

**Filter Spec**:
Factory interface for creating filter instances. Defines `Name()` and `CreateFilter(args)`.
_Avoid_: filter definition, filter type

**Filter Instance**:
A route-specific filter with bound arguments, implementing `Request(FilterContext)` and `Response(FilterContext)`.

**Filter Context**:
Request-scoped state shared among filters in a chain. Provides access to request, response, metrics, tracer, logger, state bag, path params, and loopback/serve controls.
_Avoid_: context, ctx

**Filter Chain**:
Sequential filter execution: request-phase filters run in definition order before the backend call; response-phase filters run in reverse order after.

**Filter Breaking**:
Early termination of the filter chain by calling `Serve()` with a custom response.
_Avoid_: short-circuit

**State Bag**:
A `map[string]interface{}` on the filter context for sharing data between filters in the same request.

Full catalog: `docs/reference/filters.md`

## Route Processing

**DataClient**:
Abstraction for route definition sources. Implements `LoadAll()` and `LoadUpdate()` to feed routes into the routing engine.
_Avoid_: route source, provider

**PreProcessor**:
Interface that transforms `[]*eskip.Route` before the routing tree is built. The routing engine calls registered preprocessors in sequence.

**PostProcessor**:
Interface that transforms `[]*routing.Route` after the routing tree is built but before the proxy uses them.

**Editor**:
A preprocessor that regex-replaces predicates in route definitions (e.g., migrate `Source` to `ClientIP`).

**Clone**:
A preprocessor that duplicates matching routes with predicate substitutions (prefixed `clone_`), keeping originals alongside the modified copies for migration paths.

**DefaultFilters**:
A preprocessor that prepends and/or appends a common filter set to all routes.

## Load Balancing

**Load Balancing**:
Distribution of requests across multiple backend endpoints within an LB Backend route.

**RoundRobin**:
Cycles sequentially through endpoints. The default algorithm.

**Random**:
Selects an endpoint at random for each request.

**ConsistentHash**:
Routes requests to endpoints based on a hash of the client key, providing session affinity.

**PowerOfRandomNChoices**:
Picks N random endpoints (default 2), selects the one with fewest outstanding requests.

**Endpoint Registry**:
A shared store of per-endpoint runtime metrics (detection time, inflight requests) used by load balancing and fade-in. Uses passive health checking only.

**Fade-in**:
Gradually ramps traffic to newly detected endpoints over a configured duration, so fresh instances are not overwhelmed at startup.
_Avoid_: slow start, warm-up

**LB Endpoint**:
A single backend address within an LB Backend route, carrying scheme, host, port, and optional topology zone.

## Resilience

**Circuit Breaker**:
Egress fault-tolerance mechanism that short-circuits requests to a failing backend, returning 503 while open. Applied on the client (proxy) side of outgoing connections.

**Consecutive Failures Breaker**:
Opens after N failures in a row.

**Rate Breaker**:
Opens when N failures occur within a sliding window of the last M events.

**Breaker States**:
Closed (traffic flows), Open (traffic blocked for timeout), Half-Open (trial requests; any failure reverts to Open).

**Rate Limiting**:
Traffic control that bounds request rate, rejecting excess with 429.

**Service Rate Limit**:
Local per-instance limit protecting a backend from overload.
_Avoid_: global rate limit

**Client Rate Limit**:
Per-client limit (identified by a Lookuper) protecting against chatty or abusive clients.

**Cluster Rate Limit**:
Distributed limit summed across all skipper instances (via Redis, Valkey, or peer state).

**Lookuper**:
Client-identification strategy that returns the bucket key from a request. Types: `XForwardedForLookuper` (client IP from X-Forwarded-For), `HeaderLookuper` (named header value), `TupleLookuper` (combination of multiple fields), `SameBucketLookuper` (all requests share one bucket).
_Avoid_: identifier, classifier

**Load Shedding**:
Category of filters that reject traffic to prevent overload cascading. Currently: `admissionControl`. More planned.
_Avoid_: admission control (overloaded with Kubernetes admission webhooks)

**Admission Control (admissionControl filter)**:
A load shedder that probabilistically rejects requests when measured success rate drops below threshold.

**Admission Control Modes**:
- `inactive` — never rejects traffic.
- `logInactive` — never rejects, but logs computed probabilities (shadow/tuning mode).
- `active` — actively rejects traffic with 503.
_Avoid_: passive (not a real mode)

**Admission Signal Header**:
`Admission-Control: true` — set on rejected responses to signal other shedders in the call path not to count these as backend errors, preventing distributed cascading failures.

## mTLS

**Client Certificate Authentication (mtlsAuthn)**:
Verifies the client's TLS certificate chain against configured root and intermediate CA pools. Must precede any authz filter.

**mTLS Authorization Filters**:
Allow-list filters (`mtlsCN`, `mtlsIssuerDN`, `mtlsSanDNS`, `mtlsSanIP`, `mtlsSanCIDR`, `mtlsSanURI`) that inspect specific certificate fields and return 403 on mismatch.

**Inbound mTLS Termination**:
Proxy-level config (`TLSClientAuth`) requesting or requiring client certificates on incoming connections.

**Outbound mTLS to Backends**:
Proxy-level config (`EnableMTLS`) presenting a client certificate when connecting to upstream backends, with hot cert rotation.

## Kubernetes Integration

**Ingress**:
Standard Kubernetes Ingress resource converted to eskip routes by the Kubernetes dataclient.

**RouteGroup**:
Custom Skipper CRD for advanced routing beyond what Ingress supports.

**Service Discovery**:
Automatic endpoint updates from Kubernetes EndpointSlices.

**East-West Routing**:
Internal service-to-service routing within Kubernetes, bypassing the public ingress path. Routes are copied with a `Host` predicate matching `<name>.<namespace>.<eastWestDomain>`.
_Avoid_: internal routing

**East-West Domain**:
Internal domain suffix used to identify and match east-west traffic (default: `skipper.cluster.local`).

## Zone-Aware Routing

**Zone-Aware Routing**:
Distribution of traffic preferentially to endpoints within the same Kubernetes topology zone as the serving proxy instance.

**Topology Zone**:
Kubernetes zone label on an endpoint (e.g., `"eu-central-1c"`), carried in `LBEndpoint.Zone`.

**Zone Threshold**:
Minimum 3 zone-local endpoints required for zone filtering to apply. Below threshold, falls back to all endpoints across all zones.

**Zone-Aware Opt-Out**:
Annotation `zalando.org/traffic-zone-aware: "false"` on an Ingress or RouteGroup disables zone filtering for that resource. Propagated to RouteSRV via an injected `annotate(...)` marker filter.

**RouteSRV Mode**:
Central route server precomputes per-zone route sets; each proxy fetches `/routes/{its-zone}`. The dataclient keeps all endpoints with zone metadata unfiltered.

**DataClient Mode**:
Each proxy instance filters endpoints to its own zone at the dataclient layer during endpoint fetch.

## RouteSRV

**RouteSRV**:
A Skipper-specific control plane service that reduces ingress data plane work and offloads the kube-apiserver by pre-building and caching the route table for skipper instances.
_Avoid_: route server, route proxy

**ETag-based Caching**:
SHA-256 hash of the serialized route table used as the HTTP ETag. Skipper instances skip downloading unchanged tables via `If-None-Match` / 304.

## Observability

**Metrics**:
Performance and operational measurements: per-route, per-filter, per-backend, and per-host timings and counters, plus custom metrics emitted by filters via `FilterContext.Metrics()`.

**Tracing**:
Distributed tracing using the OpenTracing API internally. OpenTelemetry is supported transparently via an OT-to-OTel bridge configured at startup.
_Avoid_: OpenTelemetry directly in filter/proxy code

**Access Log**:
Per-request HTTP access logging in Apache combined or JSON format, with duration, host, and flow-id fields.
