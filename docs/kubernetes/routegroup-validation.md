# RouteGroup Operations

RouteGroup is a [Custom Resource
Definition](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/)
(CRD).

## RouteGroup Validation

CRDs can be validated at create and update time. The
validation can be done via JSON Schemas, which enables input type
validation and string validation with regular expressions.
In addition to JSON Schema you can use a custom [validation webhook](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#validatingadmissionwebhook).

For RouteGroup we provide a CRD yaml with [JSON
schema](https://github.com/zalando/skipper/blob/master/dataclients/kubernetes/deploy/apply/routegroups_crd.yaml)
and a validation [webhook](https://github.com/zalando/skipper/tree/master/cmd/webhook) as separate binary `webhook` in the same
docker container as `skipper`.

### Synopsis

```sh
% docker run registry.opensource.zalan.do/teapot/skipper:latest webhook --help
usage: webhook [<flags>]

Flags:
  --help                         Show context-sensitive help (also try --help-long and --help-man).
  --debug                        Enable debug logging
  --tls-cert-file=TLS-CERT-FILE  File containing the certificate for HTTPS
  --tls-key-file=TLS-KEY-FILE    File containing the private key for HTTPS
  --address=":9443"              The address to listen on
```

### Validation Webhook Installation

A [Kubernetes validation
webhook](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#validatingadmissionwebhook)
can be installed next to the kubernetes API server. In order to do
this you need:

1. A container running the webhook
2. A ValidatingWebhookConfiguration configuration

Kubernetes container spec for the RouteGroup validation webhook can
be installed in your kube-apiserver Pod, such that it can communicate
via localhost.

We use the TLS based [ValidatingWebhookConfiguration
configuration](https://github.com/zalando-incubator/kubernetes-on-aws/blob/dev/cluster/manifests/01-admission-control/routegroups-webhook.yaml),
that we show below, but you can also scroll down to the [Configuration
without TLS](#configuration-without-tls). The configuration will make sure the validation
webhook is called on all create and update
operations to `zalando.org/v1/routegroups` by the Kubernetes API server.

#### Configuration with TLS
Here you can see the Pod spec with enabled TLS:

```yaml
- name: routegroups-admission-webhook
  image: registry.opensource.zalan.do/teapot/skipper:v0.13.3
  args:
    - webhook
    - --address=:9085
    - --tls-cert-file=/etc/kubernetes/ssl/admission-controller.pem
    - --tls-key-file=/etc/kubernetes/ssl/admission-controller-key.pem
  lifecycle:
    preStop:
      exec:
        command: ["/bin/sh", "-c",  " sleep 60"]
  readinessProbe:
    httpGet:
      scheme: HTTPS
      path: /healthz
      port: 9085
    initialDelaySeconds: 5
    timeoutSeconds: 5
  resources:
    requests:
      cpu: 50m
      memory: 100Mi
  ports:
    - containerPort: 9085
  volumeMounts:
    - mountPath: /etc/kubernetes/ssl
      name: ssl-certs-kubernetes
      readOnly: true
```

Make sure you pass the `caBundle` and set the `url` depending where your webhook container is running.
```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: "routegroup-admitter.teapot.zalan.do"
  labels:
    application: routegroups-admission-webhook
webhooks:
  - name: "routegroup-admitter.teapot.zalan.do"
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: ["zalando.org"]
        apiVersions: ["v1"]
        resources: ["routegroups"]
    clientConfig:
      url: "https://localhost:9085/routegroups"
      caBundle: |
        ...8<....
    admissionReviewVersions: ["v1"]
    sideEffects: None
    timeoutSeconds: 5
```

#### Configuration without TLS

In case you don't need TLS, you do not need some of the configuration
shown above.

Container spec without TLS:

```yaml
- name: routegroups-admission-webhook
  image: registry.opensource.zalan.do/teapot/skipper:v0.13.3
  args:
    - webhook
    - --address=:9085
  lifecycle:
    preStop:
      exec:
        command: ["/bin/sh", "-c",  " sleep 60"]
  readinessProbe:
    httpGet:
      path: /healthz
      port: 9085
    initialDelaySeconds: 5
    timeoutSeconds: 5
  resources:
    requests:
      cpu: 50m
      memory: 100Mi
  ports:
    - containerPort: 9085
```

Validation webhook configuration without TLS:

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: "routegroup-admitter.teapot.zalan.do"
  labels:
    application: routegroups-admission-webhook
webhooks:
  - name: "routegroup-admitter.teapot.zalan.do"
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: ["zalando.org"]
        apiVersions: ["v1"]
        resources: ["routegroups"]
    clientConfig:
      url: "http://localhost:9085/routegroups"
    admissionReviewVersions: ["v1"]
    sideEffects: None
    timeoutSeconds: 5
```
