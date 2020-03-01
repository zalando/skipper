# Helm chart

We expect AWS IAM roles with names "-role" names are setup for:

- kube2iam: kube2iam-role
- kube-aws-iam-controller: kube-aws-iam-controller-role
- kube-ingress-aws-controller: kube-ingress-aws-controller-role
- external-dns: external-dns-role

See values.yaml to change installation options.

If you want to setup skipper, kube-ingress-aws-controller,
external-dns and kube2iam via helm in your test environment:

```
git clone https://github.com/zalando/skipper.git
cd skipper/packaging/helm
kubectl create -f skipper-aws/manifests/
helm template skipper-aws | kubectl create -f -
```

Before running production you should consider reading all important
[tutorials](https://opensource.zalando.com/skipper/tutorials/basics/)
and check the [operations
reference](https://opensource.zalando.com/skipper/operation/operation/).
