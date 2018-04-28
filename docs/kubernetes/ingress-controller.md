# Skipper Ingress Controller

This documentation is meant for for cluster operators and describes
how to install skipper as Ingress-Controller into your Kubernetes
Cluster.

## Why you should use skipper as ingress controller?

Baremetal loadbalancer perform really well, but the configuration is
not updated frequently and most of these installations are not meant
to rapidly change. With introducing kubernetes this will change and
there is a need of rapid changing http routers. Skipper is designed
for rapidly changing it's routing tree.

Cloud loadbalancers are fine to scale and to change, but does not
provide many features. Skipper has advanced resiliency and deployment
features, that you can use to enhance your environment. For example
ratelimits, circuitbreakers, blue-green deployments, shadow traffic
and [more](ingress-usage.md).

## What is an Ingress-Controller

Ingress-controllers are serving http requests into a kubernetes
cluster. Most of the time traffic will pass ingress got to a
kubernetes service IP which will forward the packets to kubernetes PODs
selected by the kubernetes service.
For having a successful ingress, you need to have a DNS name pointing
to some stable IP addresses that act as a loadbalancer.

Skipper as ingress-controller:

* cloud: deploy behind the cloud loadbalancer
* baremetal: deploy behind your hardware/software loadbalancer and have all skipper as members in one pool.

You would point your DNS entries to the
loadbalancer in front of skipper, for example automated using
[external-dns](https://github.com/kubernetes-incubator/external-dns).

## AWS deployment

In AWS, this could be an ALB with DNS pointing to the ALB. The ALB can
then point to an ingress-controller running on an EC2 node and uses
Kubernetes `hostnetwork` port specification in the POD spec.

![ingress-traffic-flow](../img/ingress-traffic-flow-aws.svg)

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
* service type ClusterIP has a selector that targets your PODs of your myapp deployment

TLS example:

* same as before, but you would terminate TLS on your layer 4 loadbalancer
* layer 4 loadbalancer has `1.2.3.4:443` as socket for a virtual server
* you can use an automated redirect for all port 80 requests to https with `-kubernetes-https-redirect`

# Install Skipper as ingress-controller

You should have a base understanding of [Kubernetes](https://kubernetes.io) and
[Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/).

Prerequisites: First you have to install skipper-ingress as for
example daemonset, create a deployment and a service.

We start to deploy skipper-ingress as a daemonset, use hostNetwork and
expose the TCP port 9999 on each Kubernetes worker node for incoming ingress
traffic.

    % cat skipper-ingress-ds.yaml
    apiVersion: extensions/v1beta1
    kind: DaemonSet
    metadata:
      name: skipper-ingress
      namespace: kube-system
      labels:
        application: skipper-ingress
        version: v0.9.115
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
            version: v0.9.115
            component: ingress
          annotations:
            scheduler.alpha.kubernetes.io/critical-pod: ''
        spec:
          affinity:
            nodeAffinity:
              requiredDuringSchedulingIgnoredDuringExecution:
                nodeSelectorTerms:
                - matchExpressions:
                  - key: master
                    operator: DoesNotExist
          tolerations:
          - key: CriticalAddonsOnly
            operator: Exists
          hostNetwork: true
          containers:
          - name: skipper-ingress
            image: registry.opensource.zalan.do/pathfinder/skipper:v0.9.115
            ports:
            - name: ingress-port
              containerPort: 9999
              hostPort: 9999
            args:
              - "skipper"
              - "-kubernetes"
              - "-kubernetes-in-cluster"
              - "-address=:9999"
              - "-proxy-preserve-host"
              - "-serve-host-metrics"
              - "-enable-ratelimits"
              - "-experimental-upgrade"
              - "-metrics-exp-decay-sample"
            resources:
              limits:
                cpu: 200m
                memory: 200Mi
              requests:
                cpu: 25m
                memory: 25Mi
            readinessProbe:
              httpGet:
                path: /kube-system/healthz
                port: 9999
              initialDelaySeconds: 5
              timeoutSeconds: 5


We now deploy a simple demo application serving html:

    % cat demo-deployment.yaml
    apiVersion: apps/v1beta1
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
            image: registry.opensource.zalan.do/pathfinder/skipper:v0.9.117
            args:
              - "skipper"
              - "-inline-routes"
              - "* -> inlineContent(\"<body style='color: white; background-color: green;'><h1>Hello!</h1>\") -> <shunt>"
            ports:
            - containerPort: 9090

We deploy a service type ClusterIP that we will select from ingress:

    % cat demo-svc.yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: sszuecs-demo
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
        application: sszuecs-demo

To deploy both, you have to run:

    % kubectl create -f demo-deployment.yaml
    % kubectl create -f demo-svc.yaml

Now we have a skipper-ingress running as daemonset exposing the TCP
port 9999 on each worker node, a backend application running with 2
replicas that serves some html on TCP port 9090, and we expose a
cluster service on TCP port 80. Besides skipper-ingress, deployment
and service can not be reached from outside the cluster. Now we expose
the application with Ingress to the external network:

    % cat demo-ing.yaml
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

To deploy this ingress, you have to run:

    % kubectl create -f demo-ing.yaml

Skipper will configure itself for the given ingress, such that you can test doing:

    % curl -v -H"Host: skipper-demo.<mydomain.org>" http://<nodeip>:9999/

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

## Install Skipper with enabled RBAC

If Role-Based Access Control ("RBAC") is enabled you have to create some additional
resources to enable Skipper to query the Kubernetes API.

This guide describes all necessary resources to get Skipper up and running in a
Kubernetes cluster with RBAC enabled but it's highly recommended to read the
[RBAC docs](https://kubernetes.io/docs/admin/authorization/rbac/) to get a better
understanding which permissions are delegated to Skipper within your Kubernetes cluster.

First create a new `ServiceAccount` which will be assigned to the Skipper pods:

    apiVersion: v1
    kind: ServiceAccount
    metadata:
      name:  aws-ingress-controller
      namespace: kube-system

the required permissions are defined within a `ClusterRole` resource.

_Note: It's important to use a `ClusterRole` instead of normal `Role` because otherwise Skipper could only access resources in the namespace the `Role` was created!_

ClusterRole:

    apiVersion: rbac.authorization.k8s.io/v1beta1
    kind: ClusterRole
    metadata:
      name: aws-ingress-controller
    rules:
    - apiGroups: ["extensions"]
      resources: ["ingresses", ]
      verbs: ["get", "list"]
    - apiGroups: ["extensions"]
      resources: ["ingresses/status", ]
      verbs: ["get", "list", "patch"]
    - apiGroups: [""]
      resources: ["namespaces", "services", "endpoints"]
      verbs: ["get", "list"]

This `ClusterRole` defines access to `get` and `list` all created ingresses, namespaces, services and endpoints.
Furthermore it enables Skipper to `patch` the state of an ingress (like updating the endpoint of an ingress).

To assign the defined `ClusterRole` to the previously created `ServiceAccount`
a `ClusterRoleBinding` has to be created:

ClusterRoleBinding:

    apiVersion: rbac.authorization.k8s.io/v1beta1
    kind: ClusterRoleBinding
    metadata:
      name: aws-ingress-controller
    roleRef:
      apiGroup: rbac.authorization.k8s.io
      kind: ClusterRole
      name: aws-ingress-controller
    subjects:
    - kind: ServiceAccount
      name: aws-ingress-controller
      namespace: kube-system

Last but not least the `ServiceAccount` has to be assigned to the Skipper daemonset.

daemonset:

    apiVersion: extensions/v1beta1
    kind: DaemonSet
    metadata:
      name: skipper-ingress
      namespace: kube-system
      labels:
        component: ingress
    spec:
      selector:
        matchLabels:
          component: ingress
      updateStrategy:
        type: RollingUpdate
      template:
        metadata:
          name: skipper-ingress
          labels:
            component: ingress
            application: skipper
        spec:
          hostNetwork: true
          serviceAccountName: aws-ingress-controller
          containers:
          - name: skipper-ingress
            image: registry.opensource.zalan.do/pathfinder/skipper:latest
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
              - "-address=:9999"
              - "-proxy-preserve-host"
              - "-serve-host-metrics"
              - "-enable-ratelimits"
              - "-experimental-upgrade"
              - "-metrics-exp-decay-sample"
            resources:
              limits:
                cpu: 200m
                memory: 200Mi
            readinessProbe:
              httpGet:
                path: /kube-system/healthz
                port: 9999
              initialDelaySeconds: 5
              timeoutSeconds: 5