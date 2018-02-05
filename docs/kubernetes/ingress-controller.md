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
to some stable IP addresses that act as a loadbalancer. In AWS, this
could be an ALB with DNS pointing to the ALB. The ALB can then point
to an ingress-controller running on an EC2 node and uses Kubernetes
`hostnetwork` port specification in the POD spec.
In datacenter, baremetal environments, you probably have a hardware
loadbalancer or some haproxy or nginx setup, that serves most of your
production traffic and DNS points to these endpoints.

Skipper as ingress-controller in clouds, can be deployed behind the
cloud loadbalancer. You would point your DNS entries to the cloud
loadbalancer, for example automated using
[external-dns](https://github.com/kubernetes-incubator/external-dns).

TODO: add pictures here

## Requirements

In general for one endpoint you need, a DNS A/AAAA record pointing to
one or more loadbalancer IPs. Skipper is best used behind this
loadbalancer to route and manipulate HTTP data.

TODO



## 3 Minutes Skipper in Kubernetes introduction

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
