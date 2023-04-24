# Shadow Traffic

This tutorial will show how to setup routing for shadow traffic, where one backend (main) will receive the full
traffic, while a shadowing backend (test) will receive only a certain percentage of the same traffic.

#### Used Predicates:

* [Tee](../reference/predicates.md#tee)
* [Traffic](../reference/predicates.md#traffic)

#### Used Filters:

* [teeLoopback](../reference/filters.md#teeloopback)

![Shadow Traffic Setup](../img/shadow-traffic.png)

### 1. Initial state

Before the shadow traffic, we are sending all traffic to the main backend.

```sh
main: * -> "https://main.example.org";
```

### 2. Clone the main route, handling 10% of the traffic

Before generating the shadow traffic, we create an identical clone of the main route that will handle only 10%
of the traffic, while the rest stays being handled by the main route.

```sh
main: * -> "https://main.example.org";
split: Traffic(.1) -> "https://main.example.org";
```

### 3. Prepare the route for the shadow traffic

The route introduced next won't handle directly any incoming requests, because they won't be matched by the
[Tee](../reference/predicates.md#tee) predicate, but it is prepared to send tee requests to the alternative,
'shadow' backend.

```sh
main: * -> "https://main.example.org";
split: Traffic(.1) -> "https://main.example.org";
shadow: Tee("shadow-test-1") && True() -> "https://shadow.example.org";
```

### 4. Apply the teeLoopback filter

Now we can apply the [teeLoopback](../reference/filters.md#teeloopback) filter to the 'split' route, using the
same label as we did in the [Tee](../reference/predicates.md#tee) predicate.

```sh
main: * -> "https://main.example.org";
split: Traffic(.1) -> teeLoopback("shadow-test-1") -> "https://main.example.org";
shadow: Tee("shadow-test-1") && True() -> "https://shadow.example.org";
```

*Note that as of now, we need to increase the weight of the 'shadow' route by adding the `True()` predicate in order to avoid that the 'split'
route would match the cloned request again.*

After this, the 'split' route will still send all the handled requests, 10% of the total traffic, to the main
backend, while the rest of the traffic is routed there by the 'main' route. However, the
[teeLoopback](../reference/filters.md#teeloopback) filter will also clone the traffic of the 'split' route, 10% of
the total, and reapply the routing on it, during which these requests will be matched by the
[Tee](../reference/predicates.md#tee) predicate, and sent to the shadow backend.
