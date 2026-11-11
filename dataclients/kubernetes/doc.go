/*
Package kubernetes implements Kubernetes Ingress support for Skipper.

See: http://kubernetes.io/docs/user-guide/ingress/

The package provides a Skipper DataClient implementation that can be used to access the Kubernetes API for
ingress resources and generate routes based on them. The client polls for the ingress settings, and there is no
need for a separate controller. On the other hand, it doesn't provide a full Ingress solution alone, because it
doesn't do any load balancer configuration or DNS updates. For a full Ingress solution, it is possible to use
Skipper together with Kube-ingress-aws-controller, which targets AWS and takes care of the load balancer setup
for Kubernetes Ingress.

See: https://github.com/zalando-incubator/kube-ingress-aws-controller

Both Kube-ingress-aws-controller and Skipper Kubernetes are part of the larger project, Kubernetes On AWS:

https://github.com/zalando-incubator/kubernetes-on-aws/

# Ingress shutdown by healthcheck

The Kubernetes ingress client catches TERM signals when the ProvideHealthcheck option is enabled, and reports
failing healthcheck after the signal was received. This means that, when the Ingress client is responsible for
the healthcheck of the cluster, and the Skipper process receives the TERM signal, it won't exit by itself
immediately, but will start reporting failures on healthcheck requests. Until it gets killed by the kubelet,
Skipper keeps serving the requests in this case.

# Example - Ingress

Please check https://opensource.zalando.com/skipper/kubernetes/ingress-usage/ for more information and examples.
*/
package kubernetes
