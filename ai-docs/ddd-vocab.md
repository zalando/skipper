# Skipper Domain-Driven Design Vocabulary

This vocabulary defines the domain-specific terms used in Skipper. Use these terms consistently in all communication.

## Core Entities

### Route
The fundamental entity representing a routing rule that matches incoming HTTP requests and defines processing.

- **Route Definition**: Static configuration in eskip DSL (predicates, filters, backend)
- **Route Instance**: Runtime representation with preprocessed filters and predicates
- **Route ID**: Unique identifier (e.g., "api_route", "health_check")

### Backend
Target destination for forwarded requests:

- **Network Backend**: Standard HTTP/HTTPS endpoint (e.g., "https://api.example.com")
- **Shunt Backend**: Handled internally without forwarding (`<shunt>`)
- **Loopback Backend**: Re-enters routing with modified request (`<loopback>`)
- **Dynamic Backend**: Target determined by filters at runtime (`<dynamic>`)
- **LB Backend**: Multiple endpoints with load balancing (`<roundRobin, "host1", "host2">`)

### Predicate
Matching conditions determining if a route applies to a request:

- **Path Predicate**: Exact path with wildcards (`Path("/api/*resource")`)
- **PathSubtree Predicate**: Path and all sub-paths (`PathSubtree("/admin")`)
- **PathRegexp Predicate**: Regex path matching (`PathRegexp(/\/api\/v[0-9]+\//)`)
- **Host Predicate**: Host header matching (`Host(/\.example\.com$/)`)
- **Method Predicate**: HTTP method (`Method("POST")`)
- **Header Predicate**: HTTP header matching (`Header("Accept", "application/json")`)
- **Custom Predicates**: User-defined logic (e.g., `Traffic(0.1)`, `Weight(100)`)
- **OTel Baggage Predicate**: Matches on OpenTelemetry baggage members
  - **Key-only matching**: Matches if baggage member key exists, regardless of value (`OTelBaggage("key")`)
  - **Key-value matching**: Matches if baggage member key exists AND value equals specified value (`OTelBaggage("key", "value")`)
  - **Baggage Member**: Key-value pair with optional properties, stored in request context and propagated across service boundaries

### Filter
Processing units modifying requests/responses in a pipeline:

- **Filter Spec**: Factory for creating filter instances (`Spec` interface)
- **Filter Instance**: Route-specific filter with arguments (`Filter` interface)
- **Filter Context**: Request-scoped state shared among filters (`FilterContext`)

## Domain Processes

### Route Matching
Finding the appropriate route for an incoming request:

1. **Path Tree Lookup**: Fast radix tree lookup
2. **Predicate Evaluation**: Testing non-path conditions
3. **Weight Resolution**: Handling multiple matches via weight
4. **Route Selection**: First fully matching route

### Request Processing Pipeline
Complete request handling flow:

1. **Route Matching**: Finding applicable route
2. **Filter Chain Execution**: Request-phase filters in order
3. **Backend Forwarding**: Sending request to target
4. **Response Filter Chain**: Response-phase filters in reverse
5. **Response Delivery**: Streaming to client

### Route Update Process
Dynamic configuration management:

- **Polling**: Regular checks for changes
- **Update Detection**: Identifying new/modified/deleted routes
- **Tree Reconstruction**: Building new routing tree
- **Atomic Switching**: Replacing active tree without downtime

### Load Balancing
Distribution across multiple backend endpoints:

- **Algorithm Selection**: roundRobin, random, consistentHash, powerOfRandomNChoices
- **Endpoint Health Tracking**: Monitoring availability
- **Fade-in**: Gradually ramping traffic to new endpoints

## Key Abstractions

### DataClient Interface
Abstraction for route definition sources:
- `LoadAll()`: Load all routes
- `LoadUpdate()`: Incremental route updates

### FilterSpec Interface
Factory pattern for filter creation:
- `Name()`: Filter identifier
- `CreateFilter()`: Create filter instance with args

### PredicateSpec Interface
Factory for custom predicate creation:
- `Name()`: Predicate identifier
- `Create()`: Create predicate instance with args

### FilterContext Interface
Request processing context:
- `Request()`: Access HTTP request
- `Response()`: Access HTTP response
- `StateBag()`: Share state between filters
- `PathParam()`: Extract path parameters
- `Serve()`: Short-circuit with custom response

## Domain Patterns

### Circuit Breaker
Fault tolerance for backend protection:

- **Consecutive Breaker**: Opens after N consecutive failures
- **Rate Breaker**: Opens when failure rate exceeds threshold
- **Breaker States**: Closed (normal), Open (blocking), Half-Open (testing)
- **Breaker Registry**: Manages per-host and per-route breakers

### Rate Limiting
Traffic control and protection:

- **Client Rate Limiting**: Per-client request rate control
- **Service Rate Limiting**: Backend protection limits
- **Cluster Rate Limiting**: Distributed limiting across instances
- **Rate Limit Lookupper**: Client identification strategies (IP, header, static)

### Admission Control
Load shedding based on measured success rate:

- **Passive Mode**: Measures success rate without rejecting requests
- **Active Mode**: Rejects requests when measured success rate drops below threshold
- **Admission Signal Header**: HTTP header (`Admission-Control: true`) added to rejected requests

### Filter Chain
Sequential processing pipeline:

- **Request Phase**: Process before backend call
- **Response Phase**: Process in reverse order after backend
- **Filter Breaking**: Early termination with custom response
- **State Sharing**: Communication via StateBag

## Data Client Types

### Kubernetes DataClient
Kubernetes-native route source:

- **Ingress Resources**: Standard Kubernetes ingress objects
- **RouteGroup Resources**: Custom Skipper CRD
- **Service Discovery**: Automatic endpoint discovery
- **Admission Control**: Webhook-based validation
- **East-West Routing**: Internal service-to-service routing within Kubernetes, bypassing the public ingress path; routes are cloned with a `Host` regexp matching the east-west domain (e.g., `svc.cluster.local`)
- **East-West Domain**: Internal domain suffix used to identify and match east-west traffic
- **Endpoint Conditions**: Lifecycle states on EndpointSlice entries — `Ready` (healthy, accepting connections), `Serving` (may be terminating but still handling requests), `Terminating` (being shut down; excluded from selection)

### File DataClient
Static file-based route definitions:

- **Eskip Files**: Routes in eskip DSL
- **Route String**: Simple command line definitions

### routesrv
Proxy service that reduces kube-apiserver load by acting as a route cache between DataClient and Skipper instances:

- **eskipBytes**: In-memory struct holding the serialized route table (`data`), per-zone serialized routes (`zoneData`), and associated SHA-256 hashes
- **ETag-based Caching**: `sha256` hash of serialized routes used as the ETag; Skipper instances skip downloading unchanged route tables via `If-None-Match`
- **Polling**: Active loop that fetches routes from DataClient, detects changes via `hasChanged`, and persists to `eskipBytes` via `formatAndSet`

## Filter Categories

### Header Manipulation
- `setRequestHeader`, `setResponseHeader`: Add/modify headers
- `appendRequestHeader`, `appendResponseHeader`: Append to headers
- `dropRequestHeader`, `dropResponseHeader`: Remove headers
- `copyRequestHeader`, `copyResponseHeader`: Copy between request/response

### Path Manipulation
- `modPath`: Regex-based path modification
- `setPath`: Replace entire path
- `setQuery`: Modify query parameters
- `stripQuery`: Remove query parameters

### Authentication & Authorization
- `basicAuth`: HTTP Basic authentication
- `oauthTokeninfoAnyScope`: OAuth token validation
- `webhook`: External authorization service
- `jwt`: JWT token validation

### Traffic Management
- `tee`: Shadow/duplicate traffic to secondary backend
- `circuitBreaker`: Circuit breaker configuration
- `ratelimit`: Request rate limiting
- `clusterRatelimit`: Distributed rate limiting

### Response Generation
- `status`: Set response status code
- `inlineContent`: Generate response content directly
- `static`: Serve static files
- `redirect`: HTTP redirects

### Observability
- `accessLog`: Request logging configuration
- `metric`: Custom metrics collection
- `tracing`: Distributed tracing integration

## Load Balancer Algorithms

### RoundRobin
Sequential distribution with random starting point

### Random
Random selection from available endpoints

### ConsistentHash
Hash-based selection using client IP for sticky sessions

### PowerOfRandomNChoices
Selects N random endpoints, chooses one with least active requests

## Zone-Aware Routing

Distribution of traffic to endpoints within the same Kubernetes topology zone:

- **Topology Zone**: Kubernetes zone label on an endpoint (e.g., `"eu-central-1c"`), carried in `LBEndpoint.Zone`
- **Zone Discovery** (`getZones`): Deriving the set of available topology zones from route LB endpoints
- **Zone-Aware Route Filtering** (`filterRoutesByZone`): Splitting a route list into per-zone lists where each zone only contains endpoints local to that zone
- **Zone Threshold** (`minEndpointsByZone = 3`): Minimum number of zone-local endpoints required to serve a zone-filtered route
- **Threshold-Based Fallback**: When a zone has fewer than `minEndpointsByZone` endpoints, `getRouteForZone` returns the original unfiltered route with all endpoints
- **Zone-Specific Route Endpoint** (`/routes/{zone}`): The routesrv HTTP endpoint that serves pre-filtered zone-aware routes for a requested zone
- **routesrv Mode vs Direct Mode**: In routesrv mode the DataClient returns all endpoints with zone metadata unfiltered (so routesrv can do per-zone serving); in direct mode the DataClient filters by zone at ingestion time

## Route Processing Extensions

### Preprocessors and Postprocessors

Hooks that transform routes before and after the routing tree is built:

- **PreProcessor interface** (`routing.PreProcessor`): Transforms `[]*eskip.Route` before the routing tree is built
- **PostProcessor interface** (`routing.PostProcessor`): Transforms `[]*routing.Route` after the routing tree is built
- **Editor** (`eskip.Editor`): A preprocessor that regex-replaces predicates in route definitions (e.g., migrate `Source` → `ClientIP`)
- **Clone** (`eskip.Clone`): A preprocessor that duplicates routes with predicate substitutions for migration paths
- **DefaultFilters** (`eskip.DefaultFilters`): A preprocessor that appends a common filter set to all matching routes

## Infrastructure Concepts

### Endpoint Registry
Central management of backend endpoint health:

- **Health Checking**: Active and passive monitoring
- **Endpoint States**: Healthy, unhealthy, dead
- **Fade-in Support**: Gradual traffic increase for new endpoints

### Metrics Collection
Performance and operational metrics:

- **Route Metrics**: Per-route counts, latencies, error rates
- **Filter Metrics**: Individual filter performance
- **Backend Metrics**: Upstream response times and success rates
- **Custom Metrics**: User-defined measurements

### Swarm Clustering
Distributed coordination using SWIM gossip protocol:

- **Member Discovery**: Finding other Skipper instances
- **State Sharing**: Distributing rate limiting and shared state
- **Failure Detection**: Identifying failed members
- **Consensus**: Eventual consistency for distributed operations
