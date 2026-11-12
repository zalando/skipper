# 05: Keep hedging compatible with LB selection and observability

**What to build:** Make hedged requests preserve Skipper's LB algorithm, endpoint-eligibility, tracing, and health-accounting semantics while selecting a different destination for the additional attempt.

**Blocked by:** 04 — Preserve streaming and clean up losing attempts.

**Status:** ready-for-agent

**Design:** [Adaptive hedged-request filter design](../../../docs/issue-3943-hedge-design.md). Source: [Skipper issue #3943](https://github.com/zalando/skipper/issues/3943).

**Delivery:** Part of an unfinished feature on the integration branch. Do not ship the feature before ticket 08 passes.

- [ ] Exercise all built-in LB algorithms with the primary endpoint excluded from additional-attempt candidates. Reuse their existing selection rules over the remaining eligible endpoints.
- [ ] Preserve original route endpoint data needed by algorithms such as ConsistentHash, and respect existing custom-algorithm slice ownership contracts.
- [ ] Do not invoke an LB algorithm with an empty candidate set or repeatedly select until a different endpoint happens to appear.
- [ ] Fade-in and passive-health fallback behavior cannot reintroduce the primary endpoint or bypass the applicable endpoint-selection constraints.
- [ ] Inflight and request accounting are attempt-local and released correctly for both attempts. Intentional loser cancellation does not count as a backend health failure.
- [ ] HTTP 4xx/5xx responses retain the existing distinction from transport failures.
- [ ] Attempts have separate tracing and timing state; spans finish exactly once under the appropriate response ownership, including streamed winning bodies.
- [ ] Request-level results and winning backend metadata remain consistent; duplicate attempts do not become duplicate incoming requests in accounting.
- [ ] End-to-end tests cover algorithm variants, filtered/no-alternative endpoints, tracing modes, and cancellation without passive-health degradation.
- [ ] Focused race/leak checks and no-filter regression tests pass.
