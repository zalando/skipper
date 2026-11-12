# 06: Preserve state across unchanged-route updates

**What to build:** Keep an unchanged route's observed delay and earned allowance when Skipper replaces its routing table, including service-discovery updates that only add or remove endpoints.

**Blocked by:** 03 — Start delayed attempts from observed p95.

**Status:** resolved

**Design:** [Adaptive hedged-request filter design](../../../docs/issue-3943-hedge-design.md). Source: [Skipper issue #3943](https://github.com/zalando/skipper/issues/3943).

**Delivery:** Part of an unfinished feature on the integration branch. Do not ship the feature before ticket 08 passes.

- [ ] A route retains the same adaptive history and budget across routine replacements when its Route ID and the rest of its Route Definition are unchanged.
- [ ] Adding or removing endpoints alone preserves adaptive state and does not restart the 100-sample observation period.
- [ ] A change to the rest of the Route Definition resets history and allowance; the changed route must observe 100 samples again before delayed hedging.
- [ ] Different routes never share adaptive state merely because their filter arguments or backend endpoints match.
- [ ] Concurrent requests using old and new Route Instances can safely use retained state during Atomic Switching.
- [ ] A request associated with reset state cannot contaminate the new route's state through a stale completion.
- [ ] Use the existing route-association/postprocessing patterns rather than changing routing lifecycle management generally.
- [ ] End-to-end update tests verify continued hedging/allowance for unchanged routes and fresh observation/allowance after meaningful changes.

## Comments

Implemented and accepted on the independent lifecycle branch after ticket 03. Public filter/postprocessor tests verify retained history and budget across endpoint updates, definition-change resets, Route ID isolation, deletion/recreation, and stale-completion isolation. Lead reran hedge/builtin tests successfully. Focused race/vet checks, root compilation, priority-route baseline, formatting, and whitespace checks passed. Idle reset and priority-population isolation remain ticket 07 work; integrated HTTP update checks remain part of ticket 08.
