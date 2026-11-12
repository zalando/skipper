# 07: Reclaim and reset route state safely

**What to build:** Make idle and recreated routes start with fresh adaptive state, and reclaim registry ownership of deleted routes without disrupting in-flight requests or other route populations.

**Blocked by:** 06 — Preserve state across unchanged-route updates.

**Status:** resolved

**Design:** [Adaptive hedged-request filter design](../../../docs/issue-3943-hedge-design.md). Source: [Skipper issue #3943](https://github.com/zalando/skipper/issues/3943).

**Delivery:** Part of an unfinished feature on the integration branch. Do not ship the feature before ticket 08 passes.

- [ ] After one minute without eligible incoming requests, reset history and budget when the next eligible request arrives. Require 100 new samples before delayed hedging resumes.
- [ ] The idle reset uses a named default constant and needs no background timer or goroutine.
- [ ] Registry pruning removes references for deleted routes while allowing requests already holding the old state to complete safely.
- [ ] Deletion followed by recreation starts with zero allowance and an empty history, even when the Route ID is reused.
- [ ] Old requests cannot repopulate pruned registry entries or update newly reset state through stale completions.
- [ ] Normal and priority route populations cannot prune or share each other's state accidentally.
- [ ] Registry-owned state stays bounded by the relevant current route populations; old state survives only through legitimate in-flight ownership.
- [ ] Tests cover idle boundary behavior, deletion/recreation, empty route sets, priority routes, repeated updates, and concurrent old/new requests.
- [ ] Race/leak checks pass without relying on old-filter closure during routine route replacement.

## Comments

Implemented and accepted. One-minute idle reset uses request-scoped state generations, and late completions remain isolated. Independent postprocessor registries separate priority and normal route populations. Public lifecycle tests passed for idle boundaries, stale/concurrent generations, population isolation, deletion/recreation, and empty sets; focused repeated/race/vet checks passed. Lead reran hedge filter/factory/proxy tests and priority-route baseline successfully. Full integrated verification remains ticket 08 work.
