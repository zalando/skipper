# Helm chart

## Prerequisites

### IAM Policies

If using [Kube2IAM](https://github.com/jtblin/kube2iam) make sure you read their documentation on IAM roles (specifically about trust relationships) on how to create the roles needed for these policies.

If using IAM Roles for Service Accounts please take a look at the [AWS Documentation](https://docs.aws.amazon.com/eks/latest/userguide/create-service-account-iam-policy-and-role.html) on how to create the roles needed for these policies.

**external-dns**

```json
{
  "Version": "2012-10-17",
  "Statement": [
      {
          "Sid": "",
          "Effect": "Allow",
          "Action": "route53:ChangeResourceRecordSets",
          "Resource": "arn:aws:route53:::hostedzone/*"
      },
      {
          "Sid": "",
          "Effect": "Allow",
          "Action": [
              "route53:ListResourceRecordSets",
              "route53:ListHostedZones"
          ],
          "Resource": "*"
      }
  ]
}
```

**kube-ingress-aws-controller**

```json
{
  "Version": "2012-10-17",
  "Statement": [
      {
          "Sid": "",
          "Effect": "Allow",
          "Action": [
              "iam:ListServerCertificates",
              "iam:GetServerCertificate",
              "iam:CreateServiceLinkedRole",
              "elasticloadbalancingv2:*",
              "elasticloadbalancing:*",
              "ec2:DescribeVpcs",
              "ec2:DescribeSubnets",
              "ec2:DescribeSecurityGroups",
              "ec2:DescribeRouteTables",
              "ec2:DescribeInternetGateways",
              "ec2:DescribeInstances",
              "ec2:DescribeAccountAttributes",
              "cloudformation:*",
              "autoscaling:DetachLoadBalancers",
              "autoscaling:DetachLoadBalancerTargetGroups",
              "autoscaling:DescribeLoadBalancerTargetGroups",
              "autoscaling:DescribeAutoScalingGroups",
              "autoscaling:AttachLoadBalancers",
              "autoscaling:AttachLoadBalancerTargetGroups",
              "acm:ListCertificates",
              "acm:GetCertificate",
              "acm:DescribeCertificate"
          ],
          "Resource": "*"
      }
  ]
}
```

## Installation

```sh
git clone https://github.com/zalando/skipper.git
cd skipper/packaging/helm/skipper-aws
helm install --name skipper-aws .
```

## Values

| Value | Description | Default |
| --- | --- | --- |
| clusterID | The name of your Kubernetes cluster | mycluster |
| region | The region in which your Kubernetes cluster resides | eu-central-1 |
| vpa | Enable vertical pod autoscaling | false |
| namespace | Namespace to install all components into (kube-system will add reasonable `priorityClassName`) | kube-system |
| eks_iam | Enable EKS IAM with service account integration | false
| kube2iam.install | Install kube2iam | true |
| kube2iam.enable | Make use of kube2iam | true |
| kube2iam.aws_role | The kube2iam role that can assume other roles | kube2iam-role |
| kube2iam.version | The version of kube2iam to install | 0.10.7 |
| kube2iam.image | The image of kube2iam to install | registry.opensource.zalan.do/teapot/kube2iam |
| kube_aws_iam_controller.enable | Enable use of kube-aws-iam-controller | false |
| kube_aws_iam_controller.install | Enable installation of kube-aws-iam-controller | false |
| kube_aws_iam_controller.aws_role | The role used by kube-aws-iam-controller | kube-aws-iam-controller-role |
| kube_aws_iam_controller.version | The version of kube_aws_iam_controller  to install | 0.10.7 |
| kube_aws_iam_controller.image | The image of kube_aws_iam_controller to install | registry.opensource.zalan.do/teapot/kube2iam |
| external_dns.ownership_prefix | The txt prefix to use for DNS entries | skipper-test |
| external_dns.aws_role | The role created for the external-dns IAM policy | external-dns-role |
| external_dns.version | The version of external-dns to use | 0.5.18 |
| external_dns.image | The image of external-dns to use | registry.opensource.zalan.do/teapot/external-dns |
| kube_ingress_aws_controller.idle_timeout | The idle connection timeout for kube-ingress-aws-controller | 60s |
| kube_ingress_aws_controller.aws_role | The role used by kube-ingress-aws-controller | kube-ingress-aws-controller-role |
| kube_ingress_aws_controller.version | The version of kube-ingress-aws-controller to install | 0.10.1 |
| kube_ingress_aws_controller.image | The image of kube-ingress-aws-controller to install | registry.opensource.zalan.do/teapot/kube-ingress-aws-controller |
| kube_ingress_aws_controller.ssl_policy | The SSL policy to use | ELBSecurityPolicy-TLS-1-2-2017-01 |
| skipper.version | The version of Skipper to install | 0.12.0 |
| skipper.image | The image of Skipper to install | registry.opensource.zalan.do/teapot/skipper |
| skipper.cluster_ratelimit | Enable rate limiting in Skipper | false |
| skipper.redis.version | The version of Redis to install | 4.0.9-master-6 |
| skipper.redis.image | The image of Redis to install | registry.opensource.zalan.do/zmon/redis |
| skipper.svc_ip |  | 10.3.11.28 |
| skipper.east_west | Enable East-West support in Skipper | false |
| skipper.east_west_domain | The East-West domain to use | .ingress.cluster.local |

See [values.yaml](values.yaml) for more.


## Documentation

Before running production you should consider reading all important
[tutorials](https://opensource.zalando.com/skipper/tutorials/basics/)
and check the [operations
reference](https://opensource.zalando.com/skipper/operation/operation/).
