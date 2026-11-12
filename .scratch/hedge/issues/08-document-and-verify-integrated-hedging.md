# 08: Complete documentation and verify the integrated feature

**What to build:** Complete the operator-facing description of the hedge filter and verify the combined request, LB, and route-lifecycle behavior before the feature can ship. This is the integration completion gate, not a substitute for tests in earlier tickets.

**Blocked by:** 05 — Keep hedging compatible with LB selection and observability; 07 — Reclaim and reset route state safely.

**Status:** needs-info

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

### Final implementation and internal review

Documentation, public HTTP routing-update integration coverage, package documentation, and benchmarks are implemented. The legacy no-filter/ineligible path avoids the attempt wrapper allocation. Internal review found and fixed loopback opt-in leakage, a per-request allocation, and related-type grouping. Public regressions demonstrated the opt-in failure and one allocation before the fixes. Re-review: Standards 2 resolved, 0 open; Spec 1 resolved, 0 open. These are internal reviews, not maintainer approval.

### Benchmarks

Twenty samples on darwin/arm64, Go 1.27.4, compared with the starting commit. Disabled baseline: 96.09 microseconds/op, 22.17 KiB/op, 198 allocations/op. Post-fix disabled: 93.97 microseconds/op, 22.24 KiB/op, 198 allocations/op; time difference was not significant and bytes/op increased 0.28%. Enabled with no additional attempt: 105.5 microseconds/op, 31.45 KiB/op, 200 allocations/op. Budget-limited second-attempt workload: 5.732 milliseconds/op, 26.06 KiB/op, 207 allocations/op, and 0.05 additional attempts/op. Enabled cases use a scripted transport; the latter simulates fixed 5 ms backend latency and is not a production prediction. The final mechanical removal of private forwarding wrappers followed this benchmark run.

### Final verification and release blocker

On the corrected snapshot, make lint passed (including repository-wide build, vet, and staticcheck), focused hedge/loopback/retry/tracing race tests passed, and whitespace checks passed. The race-test expression matched no loadbalancer tests; the non-short full suite passed the loadbalancer package separately. The full non-short suite was attempted once with two package workers and a 60-second package timeout and failed. Container Redis/Valkey subnet connections timed out, testcontainers reaper goroutines were reported by a cache leak check, and the proxy package exceeded the total package timeout during a passive-health test. No clean broad baseline comparison was performed.

Shortcheck also failed because generated plugins were built against the earlier filter interface. Forced plugin rebuilding subsequently passed all plugin-loading tests. The exact proxy passive-health subtest where the package timed out passed in isolation with a 120-second allowance (17.7 seconds). This resolves the stale-plugin artifact and isolated-test concerns, but does not make the broad suites green.

Ticket 08 remains needs-info and the feature is NOT release-ready. Container networking requires the documented Colima host route, which has not been changed. Complete broad checks in a working container-network environment, with an adequate aggregate package timeout, before resolving this ticket.
