# Kubernetes Backend Deployments

## Kubernetes Race Condition problem

As described in [#652](https://github.com/zalando/skipper/issues/652),
there is a problem that exists in kubernetes, while terminating Pods.
Terminating Pods could be graceful, but the nature of distributed
environments will show failures, because not all components in the
distributed system changed already their state. When a Pod terminates,
the controller-manager has to update the `endpoints` of the kubernetes
`service`.  Additionally skipper has to get this endpoints
list. Skipper polls the kube-apiserver every `-source-poll-timeout=<ms>`,
which defaults to 3000.
Reducing this interval or implementing watch will only reduce the
timeframe, but not fix the underlying race condition.

Mitigation strategies can be different and the next section document
strategies for application developers to mitigate the problem.

## Teardown strategies

An application that is target of an ingress can circumvent HTTP code
504s Gateway Timeouts with these strategies:

1. use [Pod lifecycle hooks](#pod-lifecycle-hooks)
2. use a SIGTERM handler to switch `readinessProbe` to unhealthy and
exit later, or just wait for SIGKILL terminating the process.

### Pod Lifecycle Hooks

[Kubernetes Pod Lifecycle
Hooks](https://kubernetes.io/docs/tasks/configure-pod-container/attach-handler-lifecycle-event/#define-poststart-and-prestop-handlers)
in the Pod spec can have a `preStop` command which executes for
example a binary. The following will execute the binary `sleep` with
argument `20` to wait 20 seconds before terminating the containers
within the Pod:

```yaml
lifecycle:
  preStop:
    exec:
      command: ["sleep","20"]
```

20 seconds should be enough to fade your Pod out of the endpoints list
and skipper's routing table.

### SIGTERM handling in Containers

An application can implement a SIGTERM handler, that changes the
`readinessProbe` target to unhealthy for the application
instance. This will make sure it will be deleted from the endpoints
list and from skipper's routing table. Similar to [Pod Lifecycle
Hooks](#pod-lifecycle-hooks) you could sleep 20 seconds and after that
terminate your application or you just wait until SIGKILL will cleanup
the instance after 60s.

```go
go func() {
    var sigs chan os.Signal
    sigs = make(chan os.Signal, 1)
    signal.Notify(sigs, syscall.SIGTERM)
    for {
        select {
            case <-sigs:
               healthCheck = unhealthy
               time.Sleep(20*time.Second)
               os.Exit(0)
        }
    }
}()
```
