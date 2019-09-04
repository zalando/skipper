# RouteGroup CRD

This document is a temporary design document, continously updated as this development branch evolves.

## Goal of the RouteGroup CRD

Provide a format for routing input that makes it easier to benefit more from Skipper's routing features, without
workarounds, than the generic Ingress specs. Primarily targeting the Kubernetes Ingress scenario.

**Goals:**

- more DRY object than ingress (hosts and backends separated from path rules)
- better support for skipper specific features
- orchestrate traffic switching for a full set of routes (RouteGroup) without redundant configuration
