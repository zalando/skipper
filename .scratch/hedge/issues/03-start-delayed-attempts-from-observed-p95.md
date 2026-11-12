# 03: Start delayed attempts from observed p95

**What to build:** On a stable routing table, let an eligible slow request start a second attempt after the route's observed response-header p95, subject to the same allowance and attempt limit as retries. This slice establishes the adaptive request race; tickets 04 and 05 complete response-resource handling and compatibility before shipping.

**Blocked by:** 02 — Add hedge eligibility and budgeted retries.

**Status:** resolved

**Design:** [Adaptive hedged-request filter design](../../../docs/issue-3943-hedge-design.md). Source: [Skipper issue #3943](https://github.com/zalando/skipper/issues/3943).

**Delivery:** Part of an unfinished feature on the integration branch. Do not ship the feature before ticket 08 passes.

- [ ] Reuse the existing bounded, exponentially decaying percentile sample directly, with existing sample-capacity and decay defaults. Do not add a dependency or estimator background goroutine.
- [ ] Each eligible incoming request that produces an HTTP response contributes exactly one sample, from first-attempt start until winning response headers. Requests without an HTTP response and losing attempts add no separate header samples.
- [ ] Collect at least 100 samples since the last reset before starting delayed attempts. Budgeted dial-failure retries remain possible during observation.
- [ ] Start a delayed attempt only while the first attempt is pending, allowance is available, a different eligible LB Endpoint exists, and the original request deadline permits it.
- [ ] First HTTP response wins, including 4xx and 5xx. A transport error does not defeat another attempt already running.
- [ ] Retries and delayed hedges share the same allowance and two-attempt limit. An eligible dial failure can use the second attempt without waiting for the adaptive delay; no third attempt is possible.
- [ ] Stop the race at response headers and retain the original request deadline.
- [ ] No delayed attempt starts for a fast first response, insufficient samples, insufficient allowance, cancellation, or no alternative endpoint.
- [ ] Deterministic end-to-end tests cover first-attempt wins, second-attempt wins, transport errors, HTTP failure responses, and deadline/attempt-limit cases. Do not rely on long sleeps for the adaptive threshold.
- [ ] Preserve attempt-local mutable state from ticket 01.

## Comments

Implemented and accepted for the integration branch. Public filter/proxy TDD covered sample readiness, exactly one logical response sample, delayed attempts, first HTTP response, transport errors, and deadlines. Lead reran hedge, retry, and retry-tracing tests successfully. Focused repeated/race tests, vet, formatting, and whitespace checks passed. Streaming/loser cleanup and health-accounting validation continue in tickets 04 and 05; route continuity remains deferred. Final integrated verification remains in ticket 08.
