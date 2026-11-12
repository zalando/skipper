# 04: Preserve streaming and clean up losing attempts

**What to build:** Return the winning response as a normal streaming response while canceling and releasing every losing attempt, including responses that arrive after the winner has already been selected.

**Blocked by:** 03 — Start delayed attempts from observed p95.

**Status:** resolved

**Design:** [Adaptive hedged-request filter design](../../../docs/issue-3943-hedge-design.md). Source: [Skipper issue #3943](https://github.com/zalando/skipper/issues/3943).

**Delivery:** Part of an unfinished feature on the integration branch. Do not ship the feature before ticket 08 passes.

- [ ] Cancel the losing attempt promptly after a winner is selected.
- [ ] Close losing response bodies, including late responses and near-simultaneous completions, without waiting for a loser before returning the winner.
- [ ] Keep the winning attempt context and body usable until normal response completion or caller cancellation; canceling the loser must not interrupt the winning stream.
- [ ] Stream the winning body without buffering the complete response and never switch responses after headers.
- [ ] Caller cancellation and the existing deadline stop outstanding work without extending the request lifetime.
- [ ] Tests exercise a slow streamed winner, a stalled loser, late and simultaneous responses, early client closure, and cancellation while attempts are active.
- [ ] Focused race and existing leak checks show no stranded attempt/result-handling goroutines or unclosed response bodies for context-respecting transports.
- [ ] Protocol upgrades continue to bypass hedging and preserve their existing bidirectional behavior.

## Comments

Accepted. The ticket 03 coordinator already met the tested ownership requirements, so this slice added public HTTP regression tests rather than production changes. Repeated tests cover winning streams, stalled/late/simultaneous losers, exactly-once body/span cleanup, client cancellation/early closure, and bidirectional upgrades. Lead reran all hedge proxy tests successfully. Focused race/vet checks passed. No red failure was claimed for already-correct behavior. Backend health accounting remains ticket 05 work.
