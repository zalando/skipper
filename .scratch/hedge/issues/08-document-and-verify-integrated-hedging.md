# 08: Complete documentation and verify the integrated feature

**What to build:** Complete the operator-facing description of the hedge filter and verify the combined request, LB, and route-lifecycle behavior before the feature can ship. This is the integration completion gate, not a substitute for tests in earlier tickets.

**Blocked by:** 05 — Keep hedging compatible with LB selection and observability; 07 — Reclaim and reset route state safely.

**Status:** ready-for-agent

**Design:** [Adaptive hedged-request filter design](../../../docs/issue-3943-hedge-design.md). Source: [Skipper issue #3943](https://github.com/zalando/skipper/issues/3943).

**Delivery:** Part of an unfinished feature on the integration branch. Do not ship the feature before ticket 08 passes.

- [ ] Document the zero-argument filter, LB-only support, bodyless GET/HEAD eligibility, safe-method assumption, upgrade bypass, and unsupported-case behavior.
- [ ] Document p95 header timing, the 100-sample threshold, time-biased rather than strict-window history, idle reset, and route-update/reset behavior.
- [ ] Document the initially zero 5% allowance capped at one attempt, its use by retries and hedges, and the two-attempt total limit.
- [ ] Explain first-HTTP-response selection including 4xx/5xx, transport-error handling, original deadlines, and the inability to switch after headers or protect against later body stalls.
- [ ] Add agreed domain definitions in the existing glossary format and package/exported Go documentation. Follow the approved comment policy; use neutral examples and fixtures.
- [ ] Verify every agreed design requirement against integrated tests, including combinations of route updates, cancellation, retries, and delayed attempts.
- [ ] Run race and existing leak checks for affected packages.
- [ ] Benchmark the disabled path, enabled requests without a hedge, and second-attempt overhead; report time, bytes, and allocations per operation and compare with benchstat against the baseline.
- [ ] Formatting, lint, and repository short checks pass. Record actual results and any environment blockers without claiming unrun checks passed.
- [ ] Confirm no new hedging dependency, general-purpose framework, unrelated cleanup, or routing-lifecycle refactor was introduced.
- [ ] Record completion evidence for both dependency branches before marking the feature ready to ship.

## Comments

Pre-ticket-05 checkpoint: formatting, whitespace checks, and make lint passed (including repository-wide vet and staticcheck). Make shortcheck was attempted with the documented Colima socket environment but terminated after about 290 seconds. Container-subnet connectivity failures and loopback address exhaustion occurred; there was no clean baseline comparison, so this does not prove the broad failures are unrelated to hedging. Hedge and builtin packages passed within that run; proxy completion was not observed. Test processes and test-session containers were cleaned up. The checkpoint is unfinished and is not release-ready.
