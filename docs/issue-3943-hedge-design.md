# Adaptive hedged-request filter design

Issue: [#3943 — Adaptive hedge request filter](https://github.com/zalando/skipper/issues/3943)

Status: Design agreed and implementation committed in stages. Release verification remains blocked; see local ticket 08.

## Goal

Add `hedge()`, an opt-in Eskip filter that reduces response-header tail latency by starting a second backend attempt when the first attempt is slow. Reuse existing components and keep Skipper-owned code small. Do not add a general-purpose hedging framework or a new hedging dependency.

## Request behavior

`hedge()` enables adaptive hedging for bodyless GET and HEAD requests to LB Backends. Other requests, protocol upgrades, and unsupported backends retain their existing behavior.

For an eligible request:

1. Start the first backend attempt normally.
2. Once the route has at least 100 response-header samples, use its observed p95 as the delay before a second attempt.
3. Start the second attempt only when:
   - the first has not completed;
   - a different eligible LB Endpoint exists;
   - the route's budget permits it; and
   - the request deadline has not expired.
4. Return the first HTTP response, including 4xx/5xx, and cancel the loser. A transport error does not defeat another attempt already running.
5. Stream the winning body normally. Do not switch responses after headers.

There are at most two backend attempts total. An existing dial-failure retry uses the same attempt allowance and budget; it cannot produce a third attempt. Before the 100-sample threshold, dial-failure retries remain subject to that limit and budget.

Both attempts can execute backend work. Cancellation does not undo side effects. Routes must follow HTTP safe-method semantics; a GET endpoint that changes state is not safe merely because its method is GET.

Example route definition:

```eskip
example: Path("/items")
    -> hedge()
    -> <roundRobin, "http://127.0.0.1:9001", "http://127.0.0.1:9002">;
```

## Traffic budget

Each route earns one additional attempt per 20 eligible incoming requests:

- Initial allowance: zero.
- Maximum accumulated allowance: one additional attempt.
- Idle time earns nothing.
- Both retries and delayed hedges consume allowance.

This bounds additional attempts to 5% of eligible traffic over time, without an initial free burst. The budget is a ceiling, not a target.

## Measurement and state

Each eligible request that produces an HTTP response contributes one latency sample: time from the first attempt's start until the winning response headers arrive. Requests that end without an HTTP response contribute no header-latency sample. Losing attempts do not add separate samples.

This measures the latency produced by the filter, not backend latency without hedging. The race and measurement both end at response headers. Neither protects against a response body that stalls afterward.

Reuse the existing bounded, exponentially decaying `rcrowley/go-metrics` sample:

- Keep separate history and budget per route.
- Reuse existing sample-capacity and decay defaults.
- Preserve state across routine routing updates and endpoint membership changes when the Route ID and the rest of the route definition stay unchanged.
- Reset state for other route-definition changes or after deletion and recreation.
- After one minute without eligible traffic, reset history and budget when the next eligible request arrives. Collect 100 samples again before delayed hedging resumes.
- Prune deleted-route registry references without invalidating state held by in-flight requests.
- Do not let one route population's pruning remove live state from another, including priority routes.
- Do not create background goroutines for adaptive state.

Recent observations have more influence, but there is no strict one-minute sample cutoff. The 100-sample threshold counts observations since the last reset, not observations in a rolling window.

All defaults must be named constants at the top of the implementation file, after imports. The filter accepts no arguments.

## Implementation scope

Reuse existing components. Add no hedging dependency, dependency fork, or general-purpose framework.

- Add the filter, route-associated state, registration, and package documentation.
- Connect its state to the proxy through the existing filter-to-proxy mechanism.
- Add attempt-local coordination in the proxy. Do not run the current `makeBackendRequest` concurrently against one shared Filter Context.
- Reuse existing LB algorithms with the primary endpoint excluded, preserving endpoint-selection constraints. Detect an empty candidate set before invoking an algorithm.
- Keep tracing, endpoint accounting, and response ownership separate per attempt.
- Exclude intentional loser cancellation from backend health failures.
- Cancel losing attempts and close their response bodies. Preserve the winning response until normal response completion.
- Do not extend the existing request deadline.
- Document `hedge()` and add the agreed domain terms to `CONTEXT.md` during implementation.

Expected areas:

- `filters/hedge/`
- Filter names and built-in registration
- `proxy/`
- Startup and postprocessor registration in `skipper.go`
- `docs/reference/filters.md`
- `CONTEXT.md`

Do not perform unrelated cleanup or refactor routing lifecycle management. Preserve unrelated changes.

Use Go doc for exported identifiers and package documentation. Omit other comments, including private-function and test comments, except references explaining non-obvious issue workarounds or dependency bugs, RFC or kernel/Go constraints, or important non-obvious edge cases.

## Existing integration points

- `skipper.Options.CustomHttpRoundTripperWrap` and `proxy.Params.CustomHttpRoundTripperWrap` already expose global outbound transport wrapping. This was introduced in [PR #1458](https://github.com/zalando/skipper/pull/1458).
- `proxy/proxy.go` selects an LB Endpoint before calling the outbound transport. A transport wrapper alone does not select a different endpoint or coordinate the existing outer retry.
- The current backend-request path writes shared tracing and request metadata. It is not safe to invoke it concurrently against the same Filter Context.
- Existing LB algorithms consume a filtered endpoint slice. Keep the original route endpoint list intact where algorithms depend on it, including consistent hashing.
- Existing scheduler and API-usage-monitoring postprocessors demonstrate route-associated reuse and registry pruning.
- Old route filters are not automatically closed on routine routing-table replacement. Goroutine-free state and removal of registry references avoid depending on that cleanup.
- The existing `rcrowley/go-metrics` dependency provides bounded, synchronized, goroutine-free samples. Use a sample directly, not timer/meter machinery that starts background work.

## Dependency decision

The linked `bhope/hedge` library is a candidate from the issue, not a requirement. Inspection found no safe, unchanged fit among the three evaluated libraries:

- [`bhope/hedge`, commit `ffeae8b`](https://github.com/bhope/hedge/blob/ffeae8b0b0dee0ab8565f185486d5c1c9542e293/hedge.go): provides adaptation and racing, but retains per-host sketch goroutines without shutdown or eviction. First completion can win with a transport error. It records the winning body's first read or close while ending the race at headers.
- [`cristalhq/hedgedhttp`, commit `0f9c2fa`](https://github.com/cristalhq/hedgedhttp/blob/0f9c2fa3615d7686fcbeea025398564e94880b8d/hedged.go): provides racing but no built-in latency estimator. Inspection found successful losing responses could remain without explicit body closure.
- [`1pkg/hedgehog`, commit `b5c37a1`](https://github.com/1pkg/hedgehog/blob/b5c37a125250a1bd0e53f934f4cd8aaf903e0219/transport.go): inspection found cancellation and response-cleanup behavior unsuitable for this design.

These findings apply to the inspected revisions. They do not establish the behavior of future releases.

The agreed direction is Skipper-owned attempt coordination with existing percentile sampling and LB algorithms. This avoids adapting a dependency while retaining most of the proxy integration work.

## Verification plan

1. Filter and state tests:
   - Argument validation and request eligibility.
   - The 100-sample threshold and p95 delay.
   - Initially zero allowance, earning, consumption, and cap.
   - Idle reset, state reuse, route changes, deletion, and priority-route isolation.
2. Proxy integration tests:
   - Distinct endpoints and no alternative endpoint.
   - Fast primary and winning hedge.
   - HTTP failure responses and transport failures.
   - Deadlines and the two-attempt limit across retries and hedges.
   - Unsupported traffic retains existing behavior.
3. Resource and concurrency checks:
   - Loser cancellation and response-body closure.
   - Winning-body streaming.
   - Endpoint health accounting and tracing.
   - Race detection and existing leak checks.
4. Benchmarks:
   - Disabled path.
   - Enabled requests without a hedge.
   - Second-attempt overhead.
   - Report time, bytes, and allocations per operation; compare with `benchstat`.
5. Run focused tests and `go test -race` for affected packages, then `make fmt`, `make lint`, and `make shortcheck`.

Use neutral examples and fixtures. Do not use organization-specific accounts, URLs, or test cases.

## Investigation evidence

The issue body and discussion were read; the issue had no discussion comments at inspection time. Repository and dependency investigation was read-only.

Baseline check passed before implementation:

```sh
go test ./proxy -run 'Test(RetryAnd(Slow|Fast)Service|H2c|ProxySSLVerifyOff)' -count=1
```

The main implementation risks are shared request state, cancellation and response ownership, retry accounting, and adaptive-state lifetime across routing generations.
