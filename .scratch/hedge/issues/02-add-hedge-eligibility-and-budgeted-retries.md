# 02: Add hedge eligibility and budgeted retries

**What to build:** Let operators configure the zero-argument hedge filter. Eligible requests earn a bounded allowance for an additional backend attempt, and dial-failure retries use that allowance. Delayed hedges are not enabled in this slice.

**Blocked by:** 01 — Isolate backend-attempt state.

**Status:** resolved

**Design:** [Adaptive hedged-request filter design](../../../docs/issue-3943-hedge-design.md). Source: [Skipper issue #3943](https://github.com/zalando/skipper/issues/3943).

**Delivery:** Part of an unfinished feature on the integration branch. Do not ship the feature before ticket 08 passes.

- [ ] The hedge filter is registered and accepts no arguments; nonempty arguments are rejected.
- [ ] Only bodyless GET and HEAD requests to LB Backends enter the new behavior. Requests without the filter, unsupported methods/backends, requests with bodies, and protocol upgrades retain existing behavior.
- [ ] History and budget state are separate for different routes, including routes with identical filter arguments. State continuity across routing updates is deferred to tickets 06 and 07.
- [ ] Each eligible incoming request earns one twentieth of an additional attempt; allowance starts at zero and is capped at one additional attempt. Time alone earns nothing.
- [ ] An eligible dial-failure retry consumes allowance only when an additional attempt starts and targets a different eligible LB Endpoint. With no allowance or no alternative, no retry starts.
- [ ] There are at most two backend attempts per eligible incoming request. Unsupported traffic retains its existing retry behavior.
- [ ] Budget accounting is correct under concurrent requests and does not permit a free initial attempt or a burst from accumulated history.
- [ ] Named constants at the top of the implementation file, after imports, hold fixed defaults. Add no tuning arguments or new dependency.
- [ ] End-to-end tests demonstrate eligibility, budget exhaustion/refill, distinct retry destinations, and the combined attempt limit.

## Comments

Implemented and accepted. Public filter-factory and HTTP proxy tests passed, including red-before-green evidence for registration, retry allowance, and no-alternative behavior. Focused race checks, affected-package vet, and formatting checks passed. Lead reran the hedge, retry, h2c, TLS, and retry-tracing tests successfully. Final integrated verification remains in ticket 08.
