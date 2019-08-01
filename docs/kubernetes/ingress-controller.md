# Skipper Ingress Controller

This documentation is meant for cluster operators and describes
how to install Skipper as Ingress-Controller in your Kubernetes
Cluster.

## Why you should use Skipper as ingress controller?

Baremetal loadbalancers perform really well, but their configuration is
not updated frequently and most of the installations are not meant
for rapid change. With the introduction of Kubernetes this assumption is
no longer valid and there was a need for a HTTP router which supported
backend routes which changed very frequently. Skipper was initially designed
for a rapidly changing routing tree and subsequently used to implement
an ingress controller in Kubernetes.

Cloud loadbalancers scale well and can be updated frequently, but do not
provide many features. Skipper has advanced resiliency and deployment
features, which you can use to enhance your environment. For example,
ratelimiters, circuitbreakers, blue-green deployments, shadow traffic
and [more](ingress-usage.md).

### Comparison with other Ingress Controllers

At Zalando we chose to run [`kube-ingress-aws-controller`](https://github.com/zalando-incubator/kube-ingress-aws-controller)
with [`skipper ingress`](https://opensource.zalando.com/skipper/kubernetes/ingress-controller/)
as the target group. While AWS loadbalancers gives us features
like TLS termination, automated certificate rotation, possible [WAF](https://aws.amazon.com/waf/),
and [Security Groups](https://docs.aws.amazon.com/vpc/latest/userguide/VPC_SecurityGroups.html),
the HTTP routing capabilities are very limited. Skipper's main advantage
compared to other HTTP routers is matching and changing HTTP. Another advantage
for us and for skipper users in general is that defaults with
[kube-ingress-aws-controller](https://github.com/zalando-incubator/kube-ingress-aws-controller),
just work as you would expect.

There are a number of other ingress controllers including
[traefik](https://traefik.io/),
[nginx](https://kubernetes.github.io/ingress-nginx/),
[haproxy](https://github.com/jcmoraisjr/haproxy-ingress) or
[aws-alb-ingress-controller](https://github.com/kubernetes-sigs/aws-alb-ingress-controller).
Why not one of these?

[HAproxy](http://www.haproxy.org/) and [Nginx](https://www.nginx.com/) are well understood and
good TCP/HTTP proxies, that were built before Kubernetes. As a result, the first drawback is
their reliance on static configuration files which comes from a time when routes and their
configurations were relatively static. Secondly, the list of annotations to implement even
basic features are already quite a big list for users. Skipper was built to support dynamically
changing route configurations, which happens quite often in Kubernetes. Other advantage of
using Skipper is that we are able to easily implement automated canary deployments,
[automated blue-green deployments](https://github.com/zalando-incubator/stackset-controller)
or [shadow traffic](https://opensource.zalando.com/skipper/kubernetes/ingress-usage/#shadow-traffic).

However there are some features that have better support in `aws-alb-ingress-controller`,
`HAproxy` and `nginx`. For instance the [`sendfile()`](https://linux.die.net/man/2/sendfile)
operation. If you need to stream a large file or large amount of files, then you may want to
go for one of these options.

`aws-alb-ingress-controller` directly routes traffic to your Kubernetes services, which is
both good and bad, because it can reduce latency, but comes with the risk of depending on
kube-proxy routing. `kube-proxy` routing can take up to 30 seconds, ETCD ttl, for finding
pods from dead nodes. In Skipper we passively observe errors from endpoints and are able to
drop these from the loadbalancer members. We add these to an actively checked member pool,
which will enable endpoints if these are healthy again from skipper point of view.
Additionally the `aws-alb-ingress-controller` does not support features like ALB sharing,
or [Server Name Indication](https://tools.ietf.org/html/rfc6066#section-3) which can reduce
costs. Features like [path rewriting](https://opensource.zalando.com/skipper/kubernetes/ingress-usage/#modify-path)
are also not currently supported.

`Traefik` has a good community and support for Kubernetes. Skipper originates from
[Project Mosaic](https://www.mosaic9.org/) which was started in 2015. Back then Traefik
was not yet a mature project and still had time to go before the v1.0.0 release.
Traefik also does not currently support our [Opentracing](https://opentracing.io/) provider.
It also did not support traffic splitting when we started [stackset-controller](https://github.com/zalando-incubator/stackset-controller)
for automated traffic switching. We have also recently done significant work on running
Skipper as API gateway within Kubernetes, which could potentially help many teams that
run a many small services on Kubernetes. Skipper predicates and filters are a powerful
abstraction which can enhance the system easily.

### Comparison with service mesh

Why run Skipper and not [Istio](https://istio.io/), [Linkerd](https://linkerd.io/) or other
service-mesh solutions?

Skipper has a Kubernetes native integration, which is reliable, proven in production since
end of 2015 as of March 2019 run in 112 Kubernetes clusters at Zalando. Skipper already has
most of the features provided
by service meshes:

- [Authentication/Authorization](https://opensource.zalando.com/skipper/tutorials/auth/) in
[Kubernetes ingress](https://opensource.zalando.com/skipper/kubernetes/ingress-usage/#authorization),
and can also integrate a custom service with [webhook](https://opensource.zalando.com/skipper/reference/filters/#webhook)
- [Diagnosis tools](https://opensource.zalando.com/skipper/kubernetes/ingress-usage/#diagnosis-throttling-bandwidth-latency)
that support latency, bandwidth throttling, random content and more.
- [Rich Metrics](https://opensource.zalando.com/skipper/operation/operation/#monitoring) which
 you can enable and disable in the Prometheus format.
- [Support for different Opentracing providers](https://opensource.zalando.com/skipper/tutorials/development/#opentracing)
including jaeger, lightstep and instana
- [Ratelimits support](https://opensource.zalando.com/skipper/tutorials/ratelimit/)
with cluster ratelimits as an pending solution, which enables you to stop login attacks easily
- Connects to endpoints directly, instead of using Kubernetes services
- Retries requests, if the request can be safely retried, which is only the case if the error
happens on the TCP/IP connection establishment or a backend whose requests are defined as
idempotent.
- Simple [East-West Communication](https://opensource.zalando.com/skipper/kubernetes/east-west-usage/)
which enables proper communication paths without the need of yet another tool to do service
discovery. See how to [run skipper as API Gateway with East-West setup](https://opensource.zalando.com/skipper/kubernetes/ingress-controller/#run-as-api-gateway-with-east-west-setup),
if you want to run this powerful setup. Kubernetes, Skipper and DNS are the service discovery
in this case.
- [Blue-green deployments](https://opensource.zalando.com/skipper/kubernetes/ingress-usage/#blue-green-deployments)
 with automation if you like to use [stackset-controller](https://github.com/zalando-incubator/stackset-controller)
- [shadow-traffic](https://opensource.zalando.com/skipper/kubernetes/ingress-usage/#shadow-traffic)
 to determine if the new version is able to handle the traffic the same as the old one
- A simple way to do [A/B tests](https://opensource.zalando.com/skipper/kubernetes/ingress-usage/#ab-test)
- You are free to use cloud providers TLS terminations and certificate rotation, which is
reliable and secure. Employees cannot download private keys and certificates are certified
by a public CA. Many mTLS setups rely on insecure CA handling and are hard to debug in case of
 failure.
- We are happy to receive issues and pull requests in our repository, but if you need a feature
which can not be implemented upstream, you are also free to use skipper as a library and
create internal features to do whatever you want.

With Skipper you do not need to choose to go all-in and you are able to add features as soon
as you need or are comfortable.

## What is an Ingress-Controller?

Ingress-controllers are serving http requests into a Kubernetes
cluster. Most of the time traffic will pass ingress and go to a
Kubernetes endpoints of the respective pods.
For having a successful ingress, you need to have a DNS name pointing
to some stable IP addresses that act as a loadbalancer.

Skipper as ingress-controller:

* cloud: deploy behind the cloud loadbalancer
* baremetal: deploy behind your hardware/software loadbalancer and have all skipper as members in one pool.

You would point your DNS entries to the
loadbalancer in front of skipper, for example automated using
[external-dns](https://github.com/kubernetes-incubator/external-dns).

## Why skipper uses endpoints and not services?

Skipper does not use [Kubernetes
Services](http://kubernetes.io/docs/user-guide/services) to route
traffic to the pods. Instead it uses the Endpoints API to bypass
kube-proxy created iptables to remove overhead like conntrack entries
for iptables DNAT. Skipper can also reuse connections to Pods, such
that you have no overhead in establishing connections all the time. To
prevent errors on node failures, Skipper also does automatically
retries to another endpoint in case it gets a connection refused or
TLS handshake error to the endpoint.  Other reasons are future support
of features like session affinity, different loadbalancer
algorithms or distributed loadbalancing also known as service mesh.

## AWS deployment

In AWS, this could be an ALB with DNS pointing to the ALB. The ALB can
then point to an ingress-controller running on an EC2 node and uses
Kubernetes `hostnetwork` port specification in the Pod spec.

A logical overview of the traffic flow in AWS is shown in this picture:

![logical ingress-traffic-flow](../img/ingress-traffic-flow-aws.svg)

We described that Skipper bypasses Kubernetes Service and use directly
endpoints for [good reasons](https://opensource.zalando.com/skipper/kubernetes/ingress-controller/#why-skipper-uses-endpoints-and-not-services),
therefore the real traffic flow is shown in the next picture.
![technical ingress-traffic-flow](../img/ingress-traffic-flow-aws-technical.svg)

## Baremetal deployment

In datacenter, baremetal environments, you probably have a hardware
loadbalancer or some haproxy or nginx setup, that serves most of your
production traffic and DNS points to these endpoints. For example
`*.ingress.example.com` could point to your virtual server IPs in front
of ingress. Skippers could be used as pool members, which do the http
routing. Your loadbalancer of choice could have a wildcard certificate
for `*.ingress.example.com` and DNS for this would point to your
loadbalancer. You can also automate DNS records with
[external-dns](https://github.com/kubernetes-incubator/external-dns),
if you for example use PowerDNS as provider and have a loadbalancer
controller that modifies the status field in ingress to your
loadbalancer virtual IP.

![ingress-traffic-flow](../img/ingress-traffic-flow-baremetal.svg)

## Requirements

In general for one endpoint you need, a DNS A/AAAA record pointing to
one or more loadbalancer IPs. Skipper is best used behind this
layer 4 loadbalancer to route and manipulate HTTP data.

minimal example:

* layer 4 loadbalancer has `1.2.3.4:80` as socket for a virtual server pointing to all skipper ingress
* `*.ingress.example.com` points to 1.2.3.4
* ingress object with host entry for `myapp.ingress.example.com` targets a service type ClusterIP
* service type ClusterIP has a selector that targets your Pods of your myapp deployment

TLS example:

* same as before, but you would terminate TLS on your layer 4 loadbalancer
* layer 4 loadbalancer has `1.2.3.4:443` as socket for a virtual server
* you can use an automated redirect for all port 80 requests to https with `-kubernetes-https-redirect`
and change the default redirect code with `-kubernetes-https-redirect-code`

# Install Skipper as ingress-controller

You should have a base understanding of [Kubernetes](https://kubernetes.io) and
[Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/).

Prerequisites: First you have to install skipper-ingress as for
example daemonset, create a deployment and a service.

We start to deploy skipper-ingress as a daemonset, use hostNetwork and
expose the TCP port 9999 on each Kubernetes worker node for incoming ingress
traffic.

```yaml
# cat skipper-ingress-ds.yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: skipper-ingress
  namespace: kube-system
  labels:
    application: skipper-ingress
    version: v0.10.180
    component: ingress
spec:
  selector:
    matchLabels:
      application: skipper-ingress
  updateStrategy:
    type: RollingUpdate
  template:
    metadata:
      name: skipper-ingress
      labels:
        application: skipper-ingress
        version: v0.10.180
        component: ingress
      annotations:
        scheduler.alpha.kubernetes.io/critical-pod: ''
    spec:
      priorityClassName: system-node-critical
      tolerations:
      - key: dedicated
        operator: Exists
      nodeSelector:
        kubernetes.io/role: worker
      hostNetwork: true
      containers:
      - name: skipper-ingress
        image: registry.opensource.zalan.do/pathfinder/skipper:v0.10.180
        ports:
        - name: ingress-port
          containerPort: 9999
          hostPort: 9999
        - name: metrics-port
          containerPort: 9911
        args:
          - "skipper"
          - "-kubernetes"
          - "-kubernetes-in-cluster"
          - "-kubernetes-path-mode=path-prefix"
          - "-address=:9999"
          - "-wait-first-route-load"
          - "-proxy-preserve-host"
          - "-serve-host-metrics"
          - "-enable-ratelimits"
          - "-experimental-upgrade"
          - "-metrics-exp-decay-sample"
          - "-reverse-source-predicate"
          - "-lb-healthcheck-interval=3s"
          - "-metrics-flavour=codahale,prometheus"
          - "-enable-connection-metrics"
          - "-max-audit-body=0"
          - "-histogram-metric-buckets=.01,.025,.05,.075,.1,.2,.3,.4,.5,.75,1,2,3,4,5,7,10,15,20,30,60,120,300,600"
        resources:
          requests:
            cpu: 150m
            memory: 150Mi
        readinessProbe:
          httpGet:
            path: /kube-system/healthz
            port: 9999
          initialDelaySeconds: 5
          timeoutSeconds: 5
        securityContext:
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 1000
```

Please check, that you are using the [latest
release](https://github.com/zalando/skipper/releases/latest), we do
not maintain the **latest** tag.

We now deploy a simple demo application serving html:

```yaml
# cat demo-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: skipper-demo
spec:
  replicas: 2
  template:
    metadata:
      labels:
        application: skipper-demo
    spec:
      containers:
      - name: skipper-demo
        image: registry.opensource.zalan.do/pathfinder/skipper:v0.10.180
        args:
          - "skipper"
          - "-inline-routes"
          - "* -> inlineContent(\"<body style='color: white; background-color: green;'><h1>Hello!</h1>\") -> <shunt>"
        ports:
        - containerPort: 9090
```

We deploy a service type ClusterIP that we will select from ingress:

```yaml
# cat demo-svc.yaml
apiVersion: v1
kind: Service
metadata:
  name: skipper-demo
  labels:
    application: skipper-demo
spec:
  type: ClusterIP
  ports:
    - port: 80
      protocol: TCP
      targetPort: 9090
      name: external
  selector:
    application: skipper-demo
```

To deploy both, you have to run:

```bash
kubectl create -f demo-deployment.yaml
kubectl create -f demo-svc.yaml
```

Now we have a skipper-ingress running as daemonset exposing the TCP
port 9999 on each worker node, a backend application running with 2
replicas that serves some html on TCP port 9090, and we expose a
cluster service on TCP port 80. Besides skipper-ingress, deployment
and service can not be reached from outside the cluster. Now we expose
the application with Ingress to the external network:

```bash
# cat demo-ing.yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: skipper-demo
spec:
  rules:
  - host: skipper-demo.<mydomain.org>
    http:
      paths:
      - backend:
          serviceName: skipper-demo
          servicePort: 80
```

To deploy this ingress, you have to run:

```bash
kubectl create -f demo-ing.yaml
```

Skipper will configure itself for the given ingress, such that you can test doing:

```bash
curl -v -H"Host: skipper-demo.<mydomain.org>" http://<nodeip>:9999/
```

The next question you may ask is: how to expose this to your customers?

The answer depends on your setup and complexity requirements. In the
simplest case you could add one A record in your DNS `*.<mydomain.org>`
to your frontend loadbalancer IP that directs all traffic from `*.<mydomain.org>`
to all Kubernetes worker nodes on TCP port 9999.

A more complex setup we use in production and can be done with
something that configures your frontend loadbalancer, for example
[kube-aws-ingress-controller](https://github.com/zalando-incubator/kube-ingress-aws-controller),
and your DNS, [external-dns](https://github.com/kubernetes-incubator/external-dns)
automatically.

## Multiple skipper deployments

If you want to split for example `internal` and `public` traffic, it
might be a good choice to split your ingress deployments. Skipper has
the flag `--kubernetes-ingress-class=<string>` to only select ingress
objects that have the annotation `kubernetes.io/ingress.class` set to
`<string>`. Skipper will only create routes for ingress objects with
it's annotation or ingress objects that do not have this annotation.

The default ingress class is `skipper`, if not set. You have to create
your ingress objects with the annotation
`kubernetes.io/ingress.class: skipper` to make sure only skipper will
serve the traffic.

Example ingress:

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    kubernetes.io/ingress.class: skipper
  name: app
spec:
  rules:
  - host: app-default.example.org
    http:
      paths:
      - backend:
          serviceName: app-svc
          servicePort: 80
```

## Scoping Skipper Deployments to a Single Namespace

In some instances you might want skipper to only watch for ingress objects
created in a single namespace. This can be achieved by using
`kubernetes-namespace=<string>` where `<string>` is the Kubernetes namespace.
Specifying this option forces Skipper to look at the namespace ingresses
endpoint rather than the cluster-wide ingresses endpoint.

By default this value is an empty string (`""`) and will scope the skipper
instance to be cluster-wide, watching all `Ingress` objects across all namespaces.

## Install Skipper with enabled RBAC

If Role-Based Access Control ("RBAC") is enabled you have to create some additional
resources to enable Skipper to query the Kubernetes API.

This guide describes all necessary resources to get Skipper up and running in a
Kubernetes cluster with RBAC enabled but it's highly recommended to read the
[RBAC docs](https://kubernetes.io/docs/admin/authorization/rbac/) to get a better
understanding which permissions are delegated to Skipper within your Kubernetes cluster.

First create a new `ServiceAccount` which will be assigned to the Skipper pods:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: skipper-ingress
  namespace: kube-system
```

the required permissions are defined within a `ClusterRole` resource.

_Note: It's important to use a `ClusterRole` instead of normal `Role` because otherwise Skipper could only access resources in the namespace the `Role` was created!_

ClusterRole:

```yaml
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRole
metadata:
  name: skipper-ingress
rules:
- apiGroups: ["extensions"]
  resources: ["ingresses", ]
  verbs: ["get", "list"]
- apiGroups: [""]
  resources: ["namespaces", "services", "endpoints", "pods"]
  verbs: ["get", "list"]
```

This `ClusterRole` defines access to `get` and `list` all created ingresses, namespaces, services and endpoints.

To assign the defined `ClusterRole` to the previously created `ServiceAccount`
a `ClusterRoleBinding` has to be created:

ClusterRoleBinding:

```yaml
apiVersion: rbac.authorization.k8s.io/v1beta1
kind: ClusterRoleBinding
metadata:
  name: skipper-ingress
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: skipper-ingress
subjects:
- kind: ServiceAccount
  name: skipper-ingress
  namespace: kube-system
```

Last but not least the `ServiceAccount` has to be assigned to the Skipper daemonset.

daemonset:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: skipper-ingress
  namespace: kube-system
  labels:
    application: skipper-ingress
    version: v0.10.180
    component: ingress
spec:
  selector:
    matchLabels:
      application: skipper-ingress
  updateStrategy:
    type: RollingUpdate
  template:
    metadata:
      name: skipper-ingress
      labels:
        application: skipper-ingress
        version: v0.10.180
        component: ingress
      annotations:
        scheduler.alpha.kubernetes.io/critical-pod: ''
    spec:
      priorityClassName: system-node-critical
      tolerations:
      - key: dedicated
        operator: Exists
      nodeSelector:
        kubernetes.io/role: worker
      hostNetwork: true
      serviceAccountName: skipper-ingress
      containers:
      - name: skipper-ingress
        image: registry.opensource.zalan.do/pathfinder/skipper:v0.10.180
        ports:
        - name: ingress-port
          containerPort: 9999
          hostPort: 9999
        - name: metrics-port
          containerPort: 9911
        args:
          - "skipper"
          - "-kubernetes"
          - "-kubernetes-in-cluster"
          - "-kubernetes-path-mode=path-prefix"
          - "-address=:9999"
          - "-wait-first-route-load"
          - "-proxy-preserve-host"
          - "-serve-host-metrics"
          - "-enable-ratelimits"
          - "-experimental-upgrade"
          - "-metrics-exp-decay-sample"
          - "-reverse-source-predicate"
          - "-lb-healthcheck-interval=3s"
          - "-metrics-flavour=codahale,prometheus"
          - "-enable-connection-metrics"
          - "-max-audit-body=0"
          - "-histogram-metric-buckets=.01,.025,.05,.075,.1,.2,.3,.4,.5,.75,1,2,3,4,5,7,10,15,20,30,60,120,300,600"
        resources:
          requests:
            cpu: 150m
            memory: 150Mi
        readinessProbe:
          httpGet:
            path: /kube-system/healthz
            port: 9999
          initialDelaySeconds: 5
          timeoutSeconds: 5
        securityContext:
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 1000
```

Please check, that you are using the [latest
release](https://github.com/zalando/skipper/releases/latest), we do
not maintain the **latest** tag.

## Helm-based deployment

[Helm](https://helm.sh/) calls itself the package manager for Kubernetes and therefore take cares of the deployment of whole applications including resources like services, configurations and so on.

Skipper is also available as community contributed Helm chart in the public [quay.io](https://quay.io/repository/) registry.
The latest packaged release can be found [here](https://quay.io/application/baez/skipper).
The source code is available at [GitHub](https://github.com/baez90/skipper-helm).

The chart includes resource definitions for the following use cases:

- RBAC
- CoreOS [Prometheus-Operator](https://github.com/coreos/prometheus-operator)

As this chart is not maintained by the Skipper developers and is still under development only the basic deployment workflow is covered here.
Check the GitHub repository for all details.

To be able to deploy the chart you will need the following components:

- `helm` CLI (Install guide [here](https://github.com/kubernetes/helm))
- Helm registry plugin (available [here](https://github.com/app-registry/appr-helm-plugin))

If your environment is setup correctly you should be able to run `helm version --client` and `helm registry version quay.io` and get some information about your tooling without any error.

It is possible to deploy the chart without any further configuration like this:

    helm registry upgrade quay.io/baez/skipper -- \
        --install \
        --wait \
        "your release name e.g. skipper"

The `--wait` switch can be omitted as it only takes care that Helm is waiting until the chart is completely deployed (meaning all resources are created).

To update the deployment to a newer version the same command can be used.

If you have RBAC enabled in your Kubernetes instance you don't have to create all the previously described resources on your own but you can let Helm create them by simply adding one more switch:

    helm registry upgrade quay.io/baez/skipper -- \
        --install \
        --wait \
        --set rbac.create=true \
        "your release name e.g. skipper"

There are some more options available for customization of the chart.
Check the repository if you need more configuration possibilities.

## Run as API Gateway with East-West setup

East-West means cluster internal service-to-service communication.
For this you need to resolve DNS to skipper for an additional domain
`.skipper.cluster.local` we introduce and add HTTP routes to route to
the specified backend from your normal ingress object.

### Skipper

To enable the East-West in skipper, you need to run skipper with
`-enable-kubernetes-east-west` enabled. Skipper will duplicate all
routes with a `Host()` predicate and change it to match the host
header scheme: `<name>.<namespace>.skipper.cluster.local`.

You need also to have a kubernetes service type ClusterIP and write
down the IP (p.e. `10.3.11.28`), which you will need in CoreDNS setup.

### CoreDNS

You can create the DNS records with the `template` plugin from CoreDNS.

Corefile example:
```
.:53 {
    errors
    health
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        upstream
        fallthrough in-addr.arpa ip6.arpa
    }
    template IN A skipper.cluster.local  {
      match "^.*[.]skipper[.]cluster[.]local"
      answer "{{ .Name }} 60 IN A 10.3.11.28"
      fallthrough
    }
    prometheus :9153
    proxy . /etc/resolv.conf
    cache 30
    reload
}
```


### Usage

If the setup was done correctly, the following ingress example will
create an internal route with
`Host(/^demo[.]default[.]skipper[.]cluster[.]local)` predicate:

```
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: demo
  namespace: default
spec:
  rules:
  - host: demo.example.org
    http:
      paths:
      - backend:
          serviceName: example
          servicePort: 80
```

Your clients inside the cluster should call this example with
`demo.default.skipper.cluster.local` in their host header. Example
from inside a container:

```
curl demo.default.skipper.cluster.local
```

## Running with Cluster Ratelimits

Cluster ratelimits require a communication exchange method to build a
skipper swarm to have a shared knowledge about the requests passing
all skipper instances. To enable this feature you need to add command
line option `-enable-swarm` and `-enable-ratelimits`.
The rest depends on the implementation, that can be:

- [Redis](https://redis.io)
- [SWIM](https://www.cs.cornell.edu/projects/Quicksilver/public_pdfs/SWIM.pdf)

### Redis based

Additionally you have to add `-swarm-redis-urls` to skipper
`args:`. For example: `-swarm-redis-urls=skipper-redis-0.skipper-redis.kube-system.svc.cluster.local:6379,skipper-redis-1.skipper-redis.kube-system.svc.cluster.local:6379`.

Running skipper with `hostNetwork` in kubernetes will not be able to
resolve redis hostnames as shown in the example, if skipper does not
have `dnsPolicy: ClusterFirstWithHostNet` in it's Pod spec, see also
[DNS policy in the official Kubernetes documentation](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#pod-s-dns-policy).

This setup is considered experimental and should be carefully tested
before running it in production.

Example redis statefulset with headless service:

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  labels:
    application: skipper-redis
    version: v4.0.9
  name: skipper-redis
  namespace: kube-system
spec:
  replicas: 2
  selector:
    matchLabels:
      application: skipper-redis
  serviceName: skipper-redis
  template:
    metadata:
      labels:
        application: skipper-redis
        version: v4.0.9
    spec:
      containers:
      - image: registry.opensource.zalan.do/zmon/redis:4.0.9-master-6
        name: skipper-redis
        ports:
        - containerPort: 6379
          protocol: TCP
        readinessProbe:
          exec:
            command:
            - redis-cli
            - ping
          failureThreshold: 3
          initialDelaySeconds: 10
          periodSeconds: 60
          successThreshold: 1
          timeoutSeconds: 1
        resources:
          limits:
            cpu: 100m
            memory: 100Mi
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
---
apiVersion: v1
kind: Service
metadata:
  labels:
    application: skipper-redis
  name: skipper-redis
  namespace: kube-system
spec:
  clusterIP: None
  ports:
  - port: 6379
    protocol: TCP
    targetPort: 6379
  selector:
    application: skipper-redis
  type: ClusterIP
```



### SWIM based

[SWIM](https://www.cs.cornell.edu/projects/Quicksilver/public_pdfs/SWIM.pdf)
is a "Scalable Weakly-consistent Infection-style Process Group
Membership Protocol", which is very interesting for example to use for
cluster ratelimits. This setup is not considered stable enough to run
production, yet.

Additionally you have to add the following command line flags to
skipper's container spec `args:`:

```
-swarm-port=9990
-swarm-label-selector-key=application
-swarm-label-selector-value=skipper-ingress
-swarm-leave-timeout=5s
-swarm-max-msg-buffer=4194304
-swarm-namespace=kube-system
```

and open another port in Kubernetes and your Firewall settings to make
the communication work with TCP and UDP to the specified `swarm-port`:

```yaml
- containerPort: 9990
  hostPort: 9990
  name: swarm-port
  protocol: TCP
```
