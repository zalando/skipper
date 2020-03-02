# Helm chart

## Pre-requisites

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
| external_dns_ownership_prefix | The txt prefix to use for DNS entries | skipper-test |
| externalDnsAwsRole | The role created for the external-dns IAM policy | external-dns-role |
| externalDnsVersion | The version of external-dns to use | 0.5.18 |
| idle_timeout | The idle connection timeout for kube-ingress-aws-controller | 60s |
| install_kube_aws_iam_controller | Enable installation of kube-aws-iam-controller | false |
| install_kube2iam | Enable installation of kube2iam | true |
| kube_aws_iam_controller | Make use of kube-aws-iam-controller | false |
| kube2iam | Make use of kube2iam | true |
| kube2iamRole | The kube2iam role that can assume other roles | kube2iam-role |
| kube2iamVersion | The version of kube2iam to install | 0.10.7 |
| kubeAwsIamAwsRole | The role used by kube-aws-iam-controller | kube-aws-iam-controller-role |
| kubeIngressAwsRole | The role used by kube-uingress-aws-controller | kube-ingress-aws-controller-role |
| kubeIngressAwsVersion | The version of kube-ingress-aws-controller to install | 0.10.1 |
| region | The region in which your Kubernetes cluster resides | eu-central-1 |
| skipperClusterRatelimit | Enable rate limiting in Skipper | false |
| skipperEastWest | Enable East-West support in Skipper | false |
| skipperEastWestDomain | The East-West domain to use | .ingress.cluster.local |
| skipperSvcIP |  | 10.3.11.28 |
| skipperVersion | The version of Skipper to install | 0.11.48 |
| sslPolicy | The SSL policy to use | ELBSecurityPolicy-TLS-1-2-2017-01 |
| vpa | Enable vertical pod autoscaling | false |

See [values.yaml](values.yaml) for more.


## Documentation

Before running production you should consider reading all important
[tutorials](https://opensource.zalando.com/skipper/tutorials/basics/)
and check the [operations
reference](https://opensource.zalando.com/skipper/operation/operation/).
