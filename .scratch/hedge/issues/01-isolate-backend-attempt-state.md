# 01: Isolate backend-attempt state

**What to build:** Preserve existing proxy request, sequential retry, and response-streaming behavior while giving each backend attempt its own mutable request metadata, tracing, timing, and response ownership. This prepares the existing proxy path for concurrent attempts without enabling hedging.

**Blocked by:** None (can start immediately).

**Status:** resolved

**Design:** [Adaptive hedged-request filter design](../../../docs/issue-3943-hedge-design.md). Source: [Skipper issue #3943](https://github.com/zalando/skipper/issues/3943).

**Delivery:** Part of an unfinished feature on the integration branch. Do not ship the feature before ticket 08 passes.

- [ ] Ordinary backend requests retain their existing request mapping, response, and error behavior.
- [ ] Sequential retries retain their existing eligibility, deadline, endpoint-selection, and tracing behavior.
- [ ] Attempt metadata can be kept separate without concurrently mutating one shared Filter Context.
- [ ] The selected response remains readable through normal response streaming; span and cancellation ownership remain correct.
- [ ] Existing normal HTTP, h2c, HTTPS, upgrade, timeout, retry, and tracing regression tests pass.
- [ ] Focused race and leak checks pass.
- [ ] Record baseline time, bytes, and allocations per operation for the affected request path and compare the refactored path.
- [ ] Keep the refactoring limited to the backend-attempt seam; add no filter, dependency, or general-purpose hedging framework.

## Comments

Implemented and accepted. Existing public HTTP proxy tests cover the refactoring; an implementation-coupled helper test was removed during integration. Focused retry, timeout, tracing, h2c, and TLS tests, proxy vet, and focused race checks passed. Full proxy tests encountered unavailable rootless Docker and existing statistical fade-in failures. The retry benchmark showed increased allocations; the integrated feature benchmark must recheck the disabled path. The acceptance checkboxes remain for final integrated verification in ticket 08.
