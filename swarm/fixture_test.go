package swarm

const content = `
{"apiVersion":"v1","kind":"Endpoints","metadata":{"labels":{"application":"skipper-ingresss"},"namespace":"kube-system","name":"skipper-ingress"},"subsets":[{"addresses":[{"ip":"10.2.9.103"},{"ip":"10.2.9.104"}],"ports":[{"port":9990,"protocol":"TCP"}]}]}
`

const content2 = `
{
  "kind": "PodList",
  "apiVersion": "v1",
  "metadata": {
    "selfLink": "/api/v1/namespaces/kube-system/pods",
    "resourceVersion": "66765918"
  },
  "items": [
    {
      "metadata": {
        "name": "skipper-ingress-03zmn",
        "generateName": "skipper-ingress-",
        "namespace": "kube-system",
        "selfLink": "/api/v1/namespaces/kube-system/pods/skipper-ingress-03zmn",
        "uid": "39ebecfb-e048-11e7-aa3f-024ceb07b1d4",
        "resourceVersion": "64927577",
        "creationTimestamp": "2017-12-13T20:57:27Z",
        "labels": {
          "application": "skipper-ingress",
          "component": "ingress",
          "controller-revision-hash": "3164944834",
          "daemonset.kubernetes.io/podTemplateHash": "3334413878",
          "pod-template-generation": "24",
          "version": "v0.9.120"
        },
        "annotations": {
          "kubernetes-log-watcher/scalyr-parser": "[{\"container\": \"skipper-ingress\", \"parser\": \"skipper-access-log\"}]\n",
          "kubernetes.io/created-by": "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"DaemonSet\",\"namespace\":\"kube-system\",\"name\":\"skipper-ingress\",\"uid\":\"b3a98efd-f1ec-11e6-b375-06c0b626d9c7\",\"apiVersion\":\"extensions\",\"resourceVersion\":\"64926357\"}}\n",
          "kubernetes.io/psp": "privileged",
          "scheduler.alpha.kubernetes.io/critical-pod": ""
        },
        "ownerReferences": [
          {
            "apiVersion": "extensions/v1beta1",
            "kind": "DaemonSet",
            "name": "skipper-ingress",
            "uid": "b3a98efd-f1ec-11e6-b375-06c0b626d9c7",
            "controller": true,
            "blockOwnerDeletion": true
          }
        ]
      },
      "spec": {
        "volumes": [
          {
            "name": "default-token-wl2zc",
            "secret": {
              "secretName": "default-token-wl2zc",
              "defaultMode": 420
            }
          }
        ],
        "containers": [
          {
            "name": "skipper-ingress",
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "args": [
              "skipper",
              "-kubernetes",
              "-kubernetes-in-cluster",
              "-address=:9999",
              "-proxy-preserve-host",
              "-serve-host-metrics",
              "-enable-ratelimits",
              "-experimental-upgrade",
              "-metrics-exp-decay-sample"
            ],
            "ports": [
              {
                "name": "ingress-port",
                "hostPort": 9999,
                "containerPort": 9999,
                "protocol": "TCP"
              }
            ],
            "resources": {
              "limits": {
                "cpu": "200m",
                "memory": "200Mi"
              },
              "requests": {
                "cpu": "25m",
                "memory": "25Mi"
              }
            },
            "volumeMounts": [
              {
                "name": "default-token-wl2zc",
                "readOnly": true,
                "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount"
              }
            ],
            "readinessProbe": {
              "httpGet": {
                "path": "/kube-system/healthz",
                "port": 9999,
                "scheme": "HTTP"
              },
              "initialDelaySeconds": 5,
              "timeoutSeconds": 5,
              "periodSeconds": 10,
              "successThreshold": 1,
              "failureThreshold": 3
            },
            "terminationMessagePath": "/dev/termination-log",
            "terminationMessagePolicy": "File",
            "imagePullPolicy": "IfNotPresent",
            "securityContext": {
              "privileged": false
            }
          }
        ],
        "restartPolicy": "Always",
        "terminationGracePeriodSeconds": 30,
        "dnsPolicy": "ClusterFirst",
        "serviceAccountName": "default",
        "serviceAccount": "default",
        "nodeName": "ip-172-31-23-106.eu-central-1.compute.internal",
        "hostNetwork": true,
        "securityContext": {},
        "imagePullSecrets": [
          {
            "name": "pierone.stups.zalan.do"
          }
        ],
        "affinity": {
          "nodeAffinity": {
            "requiredDuringSchedulingIgnoredDuringExecution": {
              "nodeSelectorTerms": [
                {
                  "matchExpressions": [
                    {
                      "key": "master",
                      "operator": "DoesNotExist"
                    }
                  ]
                }
              ]
            }
          }
        },
        "schedulerName": "default-scheduler",
        "tolerations": [
          {
            "key": "CriticalAddonsOnly",
            "operator": "Exists"
          },
          {
            "key": "node.alpha.kubernetes.io/notReady",
            "operator": "Exists",
            "effect": "NoExecute"
          },
          {
            "key": "node.alpha.kubernetes.io/unreachable",
            "operator": "Exists",
            "effect": "NoExecute"
          }
        ]
      },
      "status": {
        "phase": "Running",
        "conditions": [
          {
            "type": "Initialized",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:57:32Z"
          },
          {
            "type": "Ready",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:59:52Z"
          },
          {
            "type": "PodScheduled",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:57:36Z"
          }
        ],
        "hostIP": "172.31.23.106",
        "podIP": "127.0.0.1",
        "startTime": "2017-12-13T20:57:32Z",
        "containerStatuses": [
          {
            "name": "skipper-ingress",
            "state": {
              "running": {
                "startedAt": "2017-12-13T20:57:36Z"
              }
            },
            "lastState": {},
            "ready": true,
            "restartCount": 0,
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "imageID": "docker-pullable://registry.opensource.zalan.do/pathfinder/skipper@sha256:65e409ae5b2d553181bd086cc59aaaaa19c9b5b6e9aadd3c7b8122606f03744a",
            "containerID": "docker://fe69c8b4ec6e2f99aa4177a8ce759aba25edb505689f0ba8ec48cd948c146f7a"
          }
        ],
        "qosClass": "Burstable"
      }
    },
    {
      "metadata": {
        "name": "skipper-ingress-6dk5n",
        "generateName": "skipper-ingress-",
        "namespace": "kube-system",
        "selfLink": "/api/v1/namespaces/kube-system/pods/skipper-ingress-6dk5n",
        "uid": "2f4505d2-e4de-11e7-aa3f-024ceb07b1d4",
        "resourceVersion": "66507287",
        "creationTimestamp": "2017-12-19T17:00:58Z",
        "labels": {
          "application": "skipper-ingress",
          "component": "ingress",
          "controller-revision-hash": "3164944834",
          "daemonset.kubernetes.io/podTemplateHash": "3334413878",
          "pod-template-generation": "24",
          "version": "v0.9.120"
        },
        "annotations": {
          "kubernetes-log-watcher/scalyr-parser": "[{\"container\": \"skipper-ingress\", \"parser\": \"skipper-access-log\"}]\n",
          "kubernetes.io/created-by": "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"DaemonSet\",\"namespace\":\"kube-system\",\"name\":\"skipper-ingress\",\"uid\":\"b3a98efd-f1ec-11e6-b375-06c0b626d9c7\",\"apiVersion\":\"extensions\",\"resourceVersion\":\"64927578\"}}\n",
          "kubernetes.io/psp": "privileged",
          "scheduler.alpha.kubernetes.io/critical-pod": ""
        },
        "ownerReferences": [
          {
            "apiVersion": "extensions/v1beta1",
            "kind": "DaemonSet",
            "name": "skipper-ingress",
            "uid": "b3a98efd-f1ec-11e6-b375-06c0b626d9c7",
            "controller": true,
            "blockOwnerDeletion": true
          }
        ]
      },
      "spec": {
        "volumes": [
          {
            "name": "default-token-wl2zc",
            "secret": {
              "secretName": "default-token-wl2zc",
              "defaultMode": 420
            }
          }
        ],
        "containers": [
          {
            "name": "skipper-ingress",
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "args": [
              "skipper",
              "-kubernetes",
              "-kubernetes-in-cluster",
              "-address=:9999",
              "-proxy-preserve-host",
              "-serve-host-metrics",
              "-enable-ratelimits",
              "-experimental-upgrade",
              "-metrics-exp-decay-sample"
            ],
            "ports": [
              {
                "name": "ingress-port",
                "hostPort": 9999,
                "containerPort": 9999,
                "protocol": "TCP"
              }
            ],
            "resources": {
              "limits": {
                "cpu": "200m",
                "memory": "200Mi"
              },
              "requests": {
                "cpu": "25m",
                "memory": "25Mi"
              }
            },
            "volumeMounts": [
              {
                "name": "default-token-wl2zc",
                "readOnly": true,
                "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount"
              }
            ],
            "readinessProbe": {
              "httpGet": {
                "path": "/kube-system/healthz",
                "port": 9999,
                "scheme": "HTTP"
              },
              "initialDelaySeconds": 5,
              "timeoutSeconds": 5,
              "periodSeconds": 10,
              "successThreshold": 1,
              "failureThreshold": 3
            },
            "terminationMessagePath": "/dev/termination-log",
            "terminationMessagePolicy": "File",
            "imagePullPolicy": "IfNotPresent",
            "securityContext": {
              "privileged": false
            }
          }
        ],
        "restartPolicy": "Always",
        "terminationGracePeriodSeconds": 30,
        "dnsPolicy": "ClusterFirst",
        "serviceAccountName": "default",
        "serviceAccount": "default",
        "nodeName": "ip-172-31-1-15.eu-central-1.compute.internal",
        "hostNetwork": true,
        "securityContext": {},
        "imagePullSecrets": [
          {
            "name": "pierone.stups.zalan.do"
          }
        ],
        "affinity": {
          "nodeAffinity": {
            "requiredDuringSchedulingIgnoredDuringExecution": {
              "nodeSelectorTerms": [
                {
                  "matchExpressions": [
                    {
                      "key": "master",
                      "operator": "DoesNotExist"
                    }
                  ]
                }
              ]
            }
          }
        },
        "schedulerName": "default-scheduler",
        "tolerations": [
          {
            "key": "CriticalAddonsOnly",
            "operator": "Exists"
          },
          {
            "key": "node.alpha.kubernetes.io/notReady",
            "operator": "Exists",
            "effect": "NoExecute"
          },
          {
            "key": "node.alpha.kubernetes.io/unreachable",
            "operator": "Exists",
            "effect": "NoExecute"
          }
        ]
      },
      "status": {
        "phase": "Running",
        "conditions": [
          {
            "type": "Initialized",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-19T17:01:02Z"
          },
          {
            "type": "Ready",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-19T17:03:22Z"
          },
          {
            "type": "PodScheduled",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-19T17:01:08Z"
          }
        ],
        "hostIP": "172.31.1.15",
        "podIP": "127.0.0.1",
        "startTime": "2017-12-19T17:01:02Z",
        "containerStatuses": [
          {
            "name": "skipper-ingress",
            "state": {
              "running": {
                "startedAt": "2017-12-19T17:01:07Z"
              }
            },
            "lastState": {},
            "ready": true,
            "restartCount": 0,
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "imageID": "docker-pullable://registry.opensource.zalan.do/pathfinder/skipper@sha256:65e409ae5b2d553181bd086cc59aaaaa19c9b5b6e9aadd3c7b8122606f03744a",
            "containerID": "docker://d1f8f9e1dd2bbd33e871f50b442fd236bb3e43071e4a3b5451e5f7a311c25e19"
          }
        ],
        "qosClass": "Burstable"
      }
    },
    {
      "metadata": {
        "name": "skipper-ingress-7sx08",
        "generateName": "skipper-ingress-",
        "namespace": "kube-system",
        "selfLink": "/api/v1/namespaces/kube-system/pods/skipper-ingress-7sx08",
        "uid": "59ec52dc-e043-11e7-aa3f-024ceb07b1d4",
        "resourceVersion": "64916439",
        "creationTimestamp": "2017-12-13T20:22:33Z",
        "labels": {
          "application": "skipper-ingress",
          "component": "ingress",
          "controller-revision-hash": "3164944834",
          "daemonset.kubernetes.io/podTemplateHash": "3334413878",
          "pod-template-generation": "24",
          "version": "v0.9.120"
        },
        "annotations": {
          "kubernetes-log-watcher/scalyr-parser": "[{\"container\": \"skipper-ingress\", \"parser\": \"skipper-access-log\"}]\n",
          "kubernetes.io/created-by": "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"DaemonSet\",\"namespace\":\"kube-system\",\"name\":\"skipper-ingress\",\"uid\":\"b3a98efd-f1ec-11e6-b375-06c0b626d9c7\",\"apiVersion\":\"extensions\",\"resourceVersion\":\"64915749\"}}\n",
          "kubernetes.io/psp": "privileged",
          "scheduler.alpha.kubernetes.io/critical-pod": ""
        },
        "ownerReferences": [
          {
            "apiVersion": "extensions/v1beta1",
            "kind": "DaemonSet",
            "name": "skipper-ingress",
            "uid": "b3a98efd-f1ec-11e6-b375-06c0b626d9c7",
            "controller": true,
            "blockOwnerDeletion": true
          }
        ]
      },
      "spec": {
        "volumes": [
          {
            "name": "default-token-wl2zc",
            "secret": {
              "secretName": "default-token-wl2zc",
              "defaultMode": 420
            }
          }
        ],
        "containers": [
          {
            "name": "skipper-ingress",
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "args": [
              "skipper",
              "-kubernetes",
              "-kubernetes-in-cluster",
              "-address=:9999",
              "-proxy-preserve-host",
              "-serve-host-metrics",
              "-enable-ratelimits",
              "-experimental-upgrade",
              "-metrics-exp-decay-sample"
            ],
            "ports": [
              {
                "name": "ingress-port",
                "hostPort": 9999,
                "containerPort": 9999,
                "protocol": "TCP"
              }
            ],
            "resources": {
              "limits": {
                "cpu": "200m",
                "memory": "200Mi"
              },
              "requests": {
                "cpu": "25m",
                "memory": "25Mi"
              }
            },
            "volumeMounts": [
              {
                "name": "default-token-wl2zc",
                "readOnly": true,
                "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount"
              }
            ],
            "readinessProbe": {
              "httpGet": {
                "path": "/kube-system/healthz",
                "port": 9999,
                "scheme": "HTTP"
              },
              "initialDelaySeconds": 5,
              "timeoutSeconds": 5,
              "periodSeconds": 10,
              "successThreshold": 1,
              "failureThreshold": 3
            },
            "terminationMessagePath": "/dev/termination-log",
            "terminationMessagePolicy": "File",
            "imagePullPolicy": "IfNotPresent",
            "securityContext": {
              "privileged": false
            }
          }
        ],
        "restartPolicy": "Always",
        "terminationGracePeriodSeconds": 30,
        "dnsPolicy": "ClusterFirst",
        "serviceAccountName": "default",
        "serviceAccount": "default",
        "nodeName": "ip-172-31-8-24.eu-central-1.compute.internal",
        "hostNetwork": true,
        "securityContext": {},
        "imagePullSecrets": [
          {
            "name": "pierone.stups.zalan.do"
          }
        ],
        "affinity": {
          "nodeAffinity": {
            "requiredDuringSchedulingIgnoredDuringExecution": {
              "nodeSelectorTerms": [
                {
                  "matchExpressions": [
                    {
                      "key": "master",
                      "operator": "DoesNotExist"
                    }
                  ]
                }
              ]
            }
          }
        },
        "schedulerName": "default-scheduler",
        "tolerations": [
          {
            "key": "CriticalAddonsOnly",
            "operator": "Exists"
          },
          {
            "key": "node.alpha.kubernetes.io/notReady",
            "operator": "Exists",
            "effect": "NoExecute"
          },
          {
            "key": "node.alpha.kubernetes.io/unreachable",
            "operator": "Exists",
            "effect": "NoExecute"
          }
        ]
      },
      "status": {
        "phase": "Running",
        "conditions": [
          {
            "type": "Initialized",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:22:38Z"
          },
          {
            "type": "Ready",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:23:28Z"
          },
          {
            "type": "PodScheduled",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:23:16Z"
          }
        ],
        "hostIP": "172.31.8.24",
        "podIP": "127.0.0.1",
        "startTime": "2017-12-13T20:22:38Z",
        "containerStatuses": [
          {
            "name": "skipper-ingress",
            "state": {
              "running": {
                "startedAt": "2017-12-13T20:23:16Z"
              }
            },
            "lastState": {},
            "ready": true,
            "restartCount": 0,
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "imageID": "docker-pullable://registry.opensource.zalan.do/pathfinder/skipper@sha256:65e409ae5b2d553181bd086cc59aaaaa19c9b5b6e9aadd3c7b8122606f03744a",
            "containerID": "docker://51f22f63acb322dffe863ad7861015864f14c6fd73b3e83482b64a443d3430df"
          }
        ],
        "qosClass": "Burstable"
      }
    },
    {
      "metadata": {
        "name": "skipper-ingress-bmtrt",
        "generateName": "skipper-ingress-",
        "namespace": "kube-system",
        "selfLink": "/api/v1/namespaces/kube-system/pods/skipper-ingress-bmtrt",
        "uid": "1900cfdd-e044-11e7-aa3f-024ceb07b1d4",
        "resourceVersion": "64919711",
        "creationTimestamp": "2017-12-13T20:27:53Z",
        "labels": {
          "application": "skipper-ingress",
          "component": "ingress",
          "controller-revision-hash": "3164944834",
          "daemonset.kubernetes.io/podTemplateHash": "3334413878",
          "pod-template-generation": "24",
          "version": "v0.9.120"
        },
        "annotations": {
          "kubernetes-log-watcher/scalyr-parser": "[{\"container\": \"skipper-ingress\", \"parser\": \"skipper-access-log\"}]\n",
          "kubernetes.io/created-by": "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"DaemonSet\",\"namespace\":\"kube-system\",\"name\":\"skipper-ingress\",\"uid\":\"b3a98efd-f1ec-11e6-b375-06c0b626d9c7\",\"apiVersion\":\"extensions\",\"resourceVersion\":\"64917245\"}}\n",
          "kubernetes.io/psp": "privileged",
          "scheduler.alpha.kubernetes.io/critical-pod": ""
        },
        "ownerReferences": [
          {
            "apiVersion": "extensions/v1beta1",
            "kind": "DaemonSet",
            "name": "skipper-ingress",
            "uid": "b3a98efd-f1ec-11e6-b375-06c0b626d9c7",
            "controller": true,
            "blockOwnerDeletion": true
          }
        ]
      },
      "spec": {
        "volumes": [
          {
            "name": "default-token-wl2zc",
            "secret": {
              "secretName": "default-token-wl2zc",
              "defaultMode": 420
            }
          }
        ],
        "containers": [
          {
            "name": "skipper-ingress",
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "args": [
              "skipper",
              "-kubernetes",
              "-kubernetes-in-cluster",
              "-address=:9999",
              "-proxy-preserve-host",
              "-serve-host-metrics",
              "-enable-ratelimits",
              "-experimental-upgrade",
              "-metrics-exp-decay-sample"
            ],
            "ports": [
              {
                "name": "ingress-port",
                "hostPort": 9999,
                "containerPort": 9999,
                "protocol": "TCP"
              }
            ],
            "resources": {
              "limits": {
                "cpu": "200m",
                "memory": "200Mi"
              },
              "requests": {
                "cpu": "25m",
                "memory": "25Mi"
              }
            },
            "volumeMounts": [
              {
                "name": "default-token-wl2zc",
                "readOnly": true,
                "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount"
              }
            ],
            "readinessProbe": {
              "httpGet": {
                "path": "/kube-system/healthz",
                "port": 9999,
                "scheme": "HTTP"
              },
              "initialDelaySeconds": 5,
              "timeoutSeconds": 5,
              "periodSeconds": 10,
              "successThreshold": 1,
              "failureThreshold": 3
            },
            "terminationMessagePath": "/dev/termination-log",
            "terminationMessagePolicy": "File",
            "imagePullPolicy": "IfNotPresent",
            "securityContext": {
              "privileged": false
            }
          }
        ],
        "restartPolicy": "Always",
        "terminationGracePeriodSeconds": 30,
        "dnsPolicy": "ClusterFirst",
        "serviceAccountName": "default",
        "serviceAccount": "default",
        "nodeName": "ip-172-31-19-55.eu-central-1.compute.internal",
        "hostNetwork": true,
        "securityContext": {},
        "imagePullSecrets": [
          {
            "name": "pierone.stups.zalan.do"
          }
        ],
        "affinity": {
          "nodeAffinity": {
            "requiredDuringSchedulingIgnoredDuringExecution": {
              "nodeSelectorTerms": [
                {
                  "matchExpressions": [
                    {
                      "key": "master",
                      "operator": "DoesNotExist"
                    }
                  ]
                }
              ]
            }
          }
        },
        "schedulerName": "default-scheduler",
        "tolerations": [
          {
            "key": "CriticalAddonsOnly",
            "operator": "Exists"
          },
          {
            "key": "node.alpha.kubernetes.io/notReady",
            "operator": "Exists",
            "effect": "NoExecute"
          },
          {
            "key": "node.alpha.kubernetes.io/unreachable",
            "operator": "Exists",
            "effect": "NoExecute"
          }
        ]
      },
      "status": {
        "phase": "Running",
        "conditions": [
          {
            "type": "Initialized",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:27:58Z"
          },
          {
            "type": "Ready",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:28:48Z"
          },
          {
            "type": "PodScheduled",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:28:36Z"
          }
        ],
        "hostIP": "172.31.19.55",
        "podIP": "127.0.0.1",
        "startTime": "2017-12-13T20:27:58Z",
        "containerStatuses": [
          {
            "name": "skipper-ingress",
            "state": {
              "running": {
                "startedAt": "2017-12-13T20:28:35Z"
              }
            },
            "lastState": {},
            "ready": true,
            "restartCount": 0,
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "imageID": "docker-pullable://registry.opensource.zalan.do/pathfinder/skipper@sha256:65e409ae5b2d553181bd086cc59aaaaa19c9b5b6e9aadd3c7b8122606f03744a",
            "containerID": "docker://a8108f5caaaddcd9b2b126aa94442bf0675fdf1ae320a30ad609d659e9d52117"
          }
        ],
        "qosClass": "Burstable"
      }
    },
    {
      "metadata": {
        "name": "skipper-ingress-g20js",
        "generateName": "skipper-ingress-",
        "namespace": "kube-system",
        "selfLink": "/api/v1/namespaces/kube-system/pods/skipper-ingress-g20js",
        "uid": "a7666a2b-e040-11e7-aa3f-024ceb07b1d4",
        "resourceVersion": "64910947",
        "creationTimestamp": "2017-12-13T20:03:14Z",
        "labels": {
          "application": "skipper-ingress",
          "component": "ingress",
          "controller-revision-hash": "3164944834",
          "daemonset.kubernetes.io/podTemplateHash": "3334413878",
          "pod-template-generation": "24",
          "version": "v0.9.120"
        },
        "annotations": {
          "kubernetes-log-watcher/scalyr-parser": "[{\"container\": \"skipper-ingress\", \"parser\": \"skipper-access-log\"}]\n",
          "kubernetes.io/created-by": "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"DaemonSet\",\"namespace\":\"kube-system\",\"name\":\"skipper-ingress\",\"uid\":\"b3a98efd-f1ec-11e6-b375-06c0b626d9c7\",\"apiVersion\":\"extensions\",\"resourceVersion\":\"64910262\"}}\n",
          "kubernetes.io/psp": "privileged",
          "scheduler.alpha.kubernetes.io/critical-pod": ""
        },
        "ownerReferences": [
          {
            "apiVersion": "extensions/v1beta1",
            "kind": "DaemonSet",
            "name": "skipper-ingress",
            "uid": "b3a98efd-f1ec-11e6-b375-06c0b626d9c7",
            "controller": true,
            "blockOwnerDeletion": true
          }
        ]
      },
      "spec": {
        "volumes": [
          {
            "name": "default-token-wl2zc",
            "secret": {
              "secretName": "default-token-wl2zc",
              "defaultMode": 420
            }
          }
        ],
        "containers": [
          {
            "name": "skipper-ingress",
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "args": [
              "skipper",
              "-kubernetes",
              "-kubernetes-in-cluster",
              "-address=:9999",
              "-proxy-preserve-host",
              "-serve-host-metrics",
              "-enable-ratelimits",
              "-experimental-upgrade",
              "-metrics-exp-decay-sample"
            ],
            "ports": [
              {
                "name": "ingress-port",
                "hostPort": 9999,
                "containerPort": 9999,
                "protocol": "TCP"
              }
            ],
            "resources": {
              "limits": {
                "cpu": "200m",
                "memory": "200Mi"
              },
              "requests": {
                "cpu": "25m",
                "memory": "25Mi"
              }
            },
            "volumeMounts": [
              {
                "name": "default-token-wl2zc",
                "readOnly": true,
                "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount"
              }
            ],
            "readinessProbe": {
              "httpGet": {
                "path": "/kube-system/healthz",
                "port": 9999,
                "scheme": "HTTP"
              },
              "initialDelaySeconds": 5,
              "timeoutSeconds": 5,
              "periodSeconds": 10,
              "successThreshold": 1,
              "failureThreshold": 3
            },
            "terminationMessagePath": "/dev/termination-log",
            "terminationMessagePolicy": "File",
            "imagePullPolicy": "IfNotPresent",
            "securityContext": {
              "privileged": false
            }
          }
        ],
        "restartPolicy": "Always",
        "terminationGracePeriodSeconds": 30,
        "dnsPolicy": "ClusterFirst",
        "serviceAccountName": "default",
        "serviceAccount": "default",
        "nodeName": "ip-172-31-14-136.eu-central-1.compute.internal",
        "hostNetwork": true,
        "securityContext": {},
        "imagePullSecrets": [
          {
            "name": "pierone.stups.zalan.do"
          }
        ],
        "affinity": {
          "nodeAffinity": {
            "requiredDuringSchedulingIgnoredDuringExecution": {
              "nodeSelectorTerms": [
                {
                  "matchExpressions": [
                    {
                      "key": "master",
                      "operator": "DoesNotExist"
                    }
                  ]
                }
              ]
            }
          }
        },
        "schedulerName": "default-scheduler",
        "tolerations": [
          {
            "key": "CriticalAddonsOnly",
            "operator": "Exists"
          },
          {
            "key": "node.alpha.kubernetes.io/notReady",
            "operator": "Exists",
            "effect": "NoExecute"
          },
          {
            "key": "node.alpha.kubernetes.io/unreachable",
            "operator": "Exists",
            "effect": "NoExecute"
          }
        ]
      },
      "status": {
        "phase": "Running",
        "conditions": [
          {
            "type": "Initialized",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:03:19Z"
          },
          {
            "type": "Ready",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:03:49Z"
          },
          {
            "type": "PodScheduled",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:03:44Z"
          }
        ],
        "hostIP": "172.31.14.136",
        "podIP": "127.0.0.1",
        "startTime": "2017-12-13T20:03:19Z",
        "containerStatuses": [
          {
            "name": "skipper-ingress",
            "state": {
              "running": {
                "startedAt": "2017-12-13T20:03:43Z"
              }
            },
            "lastState": {},
            "ready": true,
            "restartCount": 0,
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "imageID": "docker-pullable://registry.opensource.zalan.do/pathfinder/skipper@sha256:65e409ae5b2d553181bd086cc59aaaaa19c9b5b6e9aadd3c7b8122606f03744a",
            "containerID": "docker://3bcbf14b4dbc77c665b62f8de1303ea18225b2a89e9114c1226330ec2e80c76e"
          }
        ],
        "qosClass": "Burstable"
      }
    },
    {
      "metadata": {
        "name": "skipper-ingress-lm8sv",
        "generateName": "skipper-ingress-",
        "namespace": "kube-system",
        "selfLink": "/api/v1/namespaces/kube-system/pods/skipper-ingress-lm8sv",
        "uid": "a96af267-e042-11e7-aa3f-024ceb07b1d4",
        "resourceVersion": "64914907",
        "creationTimestamp": "2017-12-13T20:17:37Z",
        "labels": {
          "application": "skipper-ingress",
          "component": "ingress",
          "controller-revision-hash": "3164944834",
          "daemonset.kubernetes.io/podTemplateHash": "3334413878",
          "pod-template-generation": "24",
          "version": "v0.9.120"
        },
        "annotations": {
          "kubernetes-log-watcher/scalyr-parser": "[{\"container\": \"skipper-ingress\", \"parser\": \"skipper-access-log\"}]\n",
          "kubernetes.io/created-by": "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"DaemonSet\",\"namespace\":\"kube-system\",\"name\":\"skipper-ingress\",\"uid\":\"b3a98efd-f1ec-11e6-b375-06c0b626d9c7\",\"apiVersion\":\"extensions\",\"resourceVersion\":\"64914488\"}}\n",
          "kubernetes.io/psp": "privileged",
          "scheduler.alpha.kubernetes.io/critical-pod": ""
        },
        "ownerReferences": [
          {
            "apiVersion": "extensions/v1beta1",
            "kind": "DaemonSet",
            "name": "skipper-ingress",
            "uid": "b3a98efd-f1ec-11e6-b375-06c0b626d9c7",
            "controller": true,
            "blockOwnerDeletion": true
          }
        ]
      },
      "spec": {
        "volumes": [
          {
            "name": "default-token-wl2zc",
            "secret": {
              "secretName": "default-token-wl2zc",
              "defaultMode": 420
            }
          }
        ],
        "containers": [
          {
            "name": "skipper-ingress",
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "args": [
              "skipper",
              "-kubernetes",
              "-kubernetes-in-cluster",
              "-address=:9999",
              "-proxy-preserve-host",
              "-serve-host-metrics",
              "-enable-ratelimits",
              "-experimental-upgrade",
              "-metrics-exp-decay-sample"
            ],
            "ports": [
              {
                "name": "ingress-port",
                "hostPort": 9999,
                "containerPort": 9999,
                "protocol": "TCP"
              }
            ],
            "resources": {
              "limits": {
                "cpu": "200m",
                "memory": "200Mi"
              },
              "requests": {
                "cpu": "25m",
                "memory": "25Mi"
              }
            },
            "volumeMounts": [
              {
                "name": "default-token-wl2zc",
                "readOnly": true,
                "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount"
              }
            ],
            "readinessProbe": {
              "httpGet": {
                "path": "/kube-system/healthz",
                "port": 9999,
                "scheme": "HTTP"
              },
              "initialDelaySeconds": 5,
              "timeoutSeconds": 5,
              "periodSeconds": 10,
              "successThreshold": 1,
              "failureThreshold": 3
            },
            "terminationMessagePath": "/dev/termination-log",
            "terminationMessagePolicy": "File",
            "imagePullPolicy": "IfNotPresent",
            "securityContext": {
              "privileged": false
            }
          }
        ],
        "restartPolicy": "Always",
        "terminationGracePeriodSeconds": 30,
        "dnsPolicy": "ClusterFirst",
        "serviceAccountName": "default",
        "serviceAccount": "default",
        "nodeName": "ip-172-31-3-122.eu-central-1.compute.internal",
        "hostNetwork": true,
        "securityContext": {},
        "imagePullSecrets": [
          {
            "name": "pierone.stups.zalan.do"
          }
        ],
        "affinity": {
          "nodeAffinity": {
            "requiredDuringSchedulingIgnoredDuringExecution": {
              "nodeSelectorTerms": [
                {
                  "matchExpressions": [
                    {
                      "key": "master",
                      "operator": "DoesNotExist"
                    }
                  ]
                }
              ]
            }
          }
        },
        "schedulerName": "default-scheduler",
        "tolerations": [
          {
            "key": "CriticalAddonsOnly",
            "operator": "Exists"
          },
          {
            "key": "node.alpha.kubernetes.io/notReady",
            "operator": "Exists",
            "effect": "NoExecute"
          },
          {
            "key": "node.alpha.kubernetes.io/unreachable",
            "operator": "Exists",
            "effect": "NoExecute"
          }
        ]
      },
      "status": {
        "phase": "Running",
        "conditions": [
          {
            "type": "Initialized",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:17:42Z"
          },
          {
            "type": "Ready",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:18:12Z"
          },
          {
            "type": "PodScheduled",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:18:05Z"
          }
        ],
        "hostIP": "172.31.3.122",
        "podIP": "127.0.0.1",
        "startTime": "2017-12-13T20:17:42Z",
        "containerStatuses": [
          {
            "name": "skipper-ingress",
            "state": {
              "running": {
                "startedAt": "2017-12-13T20:18:04Z"
              }
            },
            "lastState": {},
            "ready": true,
            "restartCount": 0,
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "imageID": "docker-pullable://registry.opensource.zalan.do/pathfinder/skipper@sha256:65e409ae5b2d553181bd086cc59aaaaa19c9b5b6e9aadd3c7b8122606f03744a",
            "containerID": "docker://273e97f17d275ba7ea2ab7e80e376408e9f25aa8e5b68fc4310c8e773b545326"
          }
        ],
        "qosClass": "Burstable"
      }
    },
    {
      "metadata": {
        "name": "skipper-ingress-n9hz6",
        "generateName": "skipper-ingress-",
        "namespace": "kube-system",
        "selfLink": "/api/v1/namespaces/kube-system/pods/skipper-ingress-n9hz6",
        "uid": "e8044d5c-e041-11e7-aa3f-024ceb07b1d4",
        "resourceVersion": "64913535",
        "creationTimestamp": "2017-12-13T20:12:12Z",
        "labels": {
          "application": "skipper-ingress",
          "component": "ingress",
          "controller-revision-hash": "3164944834",
          "daemonset.kubernetes.io/podTemplateHash": "3334413878",
          "pod-template-generation": "24",
          "version": "v0.9.120"
        },
        "annotations": {
          "kubernetes-log-watcher/scalyr-parser": "[{\"container\": \"skipper-ingress\", \"parser\": \"skipper-access-log\"}]\n",
          "kubernetes.io/created-by": "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"DaemonSet\",\"namespace\":\"kube-system\",\"name\":\"skipper-ingress\",\"uid\":\"b3a98efd-f1ec-11e6-b375-06c0b626d9c7\",\"apiVersion\":\"extensions\",\"resourceVersion\":\"64913112\"}}\n",
          "kubernetes.io/psp": "privileged",
          "scheduler.alpha.kubernetes.io/critical-pod": ""
        },
        "ownerReferences": [
          {
            "apiVersion": "extensions/v1beta1",
            "kind": "DaemonSet",
            "name": "skipper-ingress",
            "uid": "b3a98efd-f1ec-11e6-b375-06c0b626d9c7",
            "controller": true,
            "blockOwnerDeletion": true
          }
        ]
      },
      "spec": {
        "volumes": [
          {
            "name": "default-token-wl2zc",
            "secret": {
              "secretName": "default-token-wl2zc",
              "defaultMode": 420
            }
          }
        ],
        "containers": [
          {
            "name": "skipper-ingress",
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "args": [
              "skipper",
              "-kubernetes",
              "-kubernetes-in-cluster",
              "-address=:9999",
              "-proxy-preserve-host",
              "-serve-host-metrics",
              "-enable-ratelimits",
              "-experimental-upgrade",
              "-metrics-exp-decay-sample"
            ],
            "ports": [
              {
                "name": "ingress-port",
                "hostPort": 9999,
                "containerPort": 9999,
                "protocol": "TCP"
              }
            ],
            "resources": {
              "limits": {
                "cpu": "200m",
                "memory": "200Mi"
              },
              "requests": {
                "cpu": "25m",
                "memory": "25Mi"
              }
            },
            "volumeMounts": [
              {
                "name": "default-token-wl2zc",
                "readOnly": true,
                "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount"
              }
            ],
            "readinessProbe": {
              "httpGet": {
                "path": "/kube-system/healthz",
                "port": 9999,
                "scheme": "HTTP"
              },
              "initialDelaySeconds": 5,
              "timeoutSeconds": 5,
              "periodSeconds": 10,
              "successThreshold": 1,
              "failureThreshold": 3
            },
            "terminationMessagePath": "/dev/termination-log",
            "terminationMessagePolicy": "File",
            "imagePullPolicy": "IfNotPresent",
            "securityContext": {
              "privileged": false
            }
          }
        ],
        "restartPolicy": "Always",
        "terminationGracePeriodSeconds": 30,
        "dnsPolicy": "ClusterFirst",
        "serviceAccountName": "default",
        "serviceAccount": "default",
        "nodeName": "ip-172-31-3-163.eu-central-1.compute.internal",
        "hostNetwork": true,
        "securityContext": {},
        "imagePullSecrets": [
          {
            "name": "pierone.stups.zalan.do"
          }
        ],
        "affinity": {
          "nodeAffinity": {
            "requiredDuringSchedulingIgnoredDuringExecution": {
              "nodeSelectorTerms": [
                {
                  "matchExpressions": [
                    {
                      "key": "master",
                      "operator": "DoesNotExist"
                    }
                  ]
                }
              ]
            }
          }
        },
        "schedulerName": "default-scheduler",
        "tolerations": [
          {
            "key": "CriticalAddonsOnly",
            "operator": "Exists"
          },
          {
            "key": "node.alpha.kubernetes.io/notReady",
            "operator": "Exists",
            "effect": "NoExecute"
          },
          {
            "key": "node.alpha.kubernetes.io/unreachable",
            "operator": "Exists",
            "effect": "NoExecute"
          }
        ]
      },
      "status": {
        "phase": "Running",
        "conditions": [
          {
            "type": "Initialized",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:12:17Z"
          },
          {
            "type": "Ready",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:13:07Z"
          },
          {
            "type": "PodScheduled",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:12:56Z"
          }
        ],
        "hostIP": "172.31.3.163",
        "podIP": "127.0.0.1",
        "startTime": "2017-12-13T20:12:17Z",
        "containerStatuses": [
          {
            "name": "skipper-ingress",
            "state": {
              "running": {
                "startedAt": "2017-12-13T20:12:55Z"
              }
            },
            "lastState": {},
            "ready": true,
            "restartCount": 0,
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "imageID": "docker-pullable://registry.opensource.zalan.do/pathfinder/skipper@sha256:65e409ae5b2d553181bd086cc59aaaaa19c9b5b6e9aadd3c7b8122606f03744a",
            "containerID": "docker://7fe5a157cf8a7f33e4d42e2e39f6e6885c02df355d055c943e83d253d6f94e8f"
          }
        ],
        "qosClass": "Burstable"
      }
    },
    {
      "metadata": {
        "name": "skipper-ingress-vr2tx",
        "generateName": "skipper-ingress-",
        "namespace": "kube-system",
        "selfLink": "/api/v1/namespaces/kube-system/pods/skipper-ingress-vr2tx",
        "uid": "c98d3535-e045-11e7-aa3f-024ceb07b1d4",
        "resourceVersion": "64923102",
        "creationTimestamp": "2017-12-13T20:39:59Z",
        "labels": {
          "application": "skipper-ingress",
          "component": "ingress",
          "controller-revision-hash": "3164944834",
          "daemonset.kubernetes.io/podTemplateHash": "3334413878",
          "pod-template-generation": "24",
          "version": "v0.9.120"
        },
        "annotations": {
          "kubernetes-log-watcher/scalyr-parser": "[{\"container\": \"skipper-ingress\", \"parser\": \"skipper-access-log\"}]\n",
          "kubernetes.io/created-by": "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"DaemonSet\",\"namespace\":\"kube-system\",\"name\":\"skipper-ingress\",\"uid\":\"b3a98efd-f1ec-11e6-b375-06c0b626d9c7\",\"apiVersion\":\"extensions\",\"resourceVersion\":\"64922390\"}}\n",
          "kubernetes.io/psp": "privileged",
          "scheduler.alpha.kubernetes.io/critical-pod": ""
        },
        "ownerReferences": [
          {
            "apiVersion": "extensions/v1beta1",
            "kind": "DaemonSet",
            "name": "skipper-ingress",
            "uid": "b3a98efd-f1ec-11e6-b375-06c0b626d9c7",
            "controller": true,
            "blockOwnerDeletion": true
          }
        ]
      },
      "spec": {
        "volumes": [
          {
            "name": "default-token-wl2zc",
            "secret": {
              "secretName": "default-token-wl2zc",
              "defaultMode": 420
            }
          }
        ],
        "containers": [
          {
            "name": "skipper-ingress",
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "args": [
              "skipper",
              "-kubernetes",
              "-kubernetes-in-cluster",
              "-address=:9999",
              "-proxy-preserve-host",
              "-serve-host-metrics",
              "-enable-ratelimits",
              "-experimental-upgrade",
              "-metrics-exp-decay-sample"
            ],
            "ports": [
              {
                "name": "ingress-port",
                "hostPort": 9999,
                "containerPort": 9999,
                "protocol": "TCP"
              }
            ],
            "resources": {
              "limits": {
                "cpu": "200m",
                "memory": "200Mi"
              },
              "requests": {
                "cpu": "25m",
                "memory": "25Mi"
              }
            },
            "volumeMounts": [
              {
                "name": "default-token-wl2zc",
                "readOnly": true,
                "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount"
              }
            ],
            "readinessProbe": {
              "httpGet": {
                "path": "/kube-system/healthz",
                "port": 9999,
                "scheme": "HTTP"
              },
              "initialDelaySeconds": 5,
              "timeoutSeconds": 5,
              "periodSeconds": 10,
              "successThreshold": 1,
              "failureThreshold": 3
            },
            "terminationMessagePath": "/dev/termination-log",
            "terminationMessagePolicy": "File",
            "imagePullPolicy": "IfNotPresent",
            "securityContext": {
              "privileged": false
            }
          }
        ],
        "restartPolicy": "Always",
        "terminationGracePeriodSeconds": 30,
        "dnsPolicy": "ClusterFirst",
        "serviceAccountName": "default",
        "serviceAccount": "default",
        "nodeName": "ip-172-31-10-160.eu-central-1.compute.internal",
        "hostNetwork": true,
        "securityContext": {},
        "imagePullSecrets": [
          {
            "name": "pierone.stups.zalan.do"
          }
        ],
        "affinity": {
          "nodeAffinity": {
            "requiredDuringSchedulingIgnoredDuringExecution": {
              "nodeSelectorTerms": [
                {
                  "matchExpressions": [
                    {
                      "key": "master",
                      "operator": "DoesNotExist"
                    }
                  ]
                }
              ]
            }
          }
        },
        "schedulerName": "default-scheduler",
        "tolerations": [
          {
            "key": "CriticalAddonsOnly",
            "operator": "Exists"
          },
          {
            "key": "node.alpha.kubernetes.io/notReady",
            "operator": "Exists",
            "effect": "NoExecute"
          },
          {
            "key": "node.alpha.kubernetes.io/unreachable",
            "operator": "Exists",
            "effect": "NoExecute"
          }
        ]
      },
      "status": {
        "phase": "Running",
        "conditions": [
          {
            "type": "Initialized",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:40:04Z"
          },
          {
            "type": "Ready",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:40:54Z"
          },
          {
            "type": "PodScheduled",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:40:47Z"
          }
        ],
        "hostIP": "172.31.10.160",
        "podIP": "127.0.0.1",
        "startTime": "2017-12-13T20:40:04Z",
        "containerStatuses": [
          {
            "name": "skipper-ingress",
            "state": {
              "running": {
                "startedAt": "2017-12-13T20:40:46Z"
              }
            },
            "lastState": {},
            "ready": true,
            "restartCount": 0,
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "imageID": "docker-pullable://registry.opensource.zalan.do/pathfinder/skipper@sha256:65e409ae5b2d553181bd086cc59aaaaa19c9b5b6e9aadd3c7b8122606f03744a",
            "containerID": "docker://1bc4e819a3ba2c2a33508d678331ca188f5fe0a3177f90091054faa8a1b04a6c"
          }
        ],
        "qosClass": "Burstable"
      }
    },
    {
      "metadata": {
        "name": "skipper-ingress-xwm65",
        "generateName": "skipper-ingress-",
        "namespace": "kube-system",
        "selfLink": "/api/v1/namespaces/kube-system/pods/skipper-ingress-xwm65",
        "uid": "4b40cda1-e041-11e7-aa3f-024ceb07b1d4",
        "resourceVersion": "64912185",
        "creationTimestamp": "2017-12-13T20:07:49Z",
        "labels": {
          "application": "skipper-ingress",
          "component": "ingress",
          "controller-revision-hash": "3164944834",
          "daemonset.kubernetes.io/podTemplateHash": "3334413878",
          "pod-template-generation": "24",
          "version": "v0.9.120"
        },
        "annotations": {
          "kubernetes-log-watcher/scalyr-parser": "[{\"container\": \"skipper-ingress\", \"parser\": \"skipper-access-log\"}]\n",
          "kubernetes.io/created-by": "{\"kind\":\"SerializedReference\",\"apiVersion\":\"v1\",\"reference\":{\"kind\":\"DaemonSet\",\"namespace\":\"kube-system\",\"name\":\"skipper-ingress\",\"uid\":\"b3a98efd-f1ec-11e6-b375-06c0b626d9c7\",\"apiVersion\":\"extensions\",\"resourceVersion\":\"64911800\"}}\n",
          "kubernetes.io/psp": "privileged",
          "scheduler.alpha.kubernetes.io/critical-pod": ""
        },
        "ownerReferences": [
          {
            "apiVersion": "extensions/v1beta1",
            "kind": "DaemonSet",
            "name": "skipper-ingress",
            "uid": "b3a98efd-f1ec-11e6-b375-06c0b626d9c7",
            "controller": true,
            "blockOwnerDeletion": true
          }
        ]
      },
      "spec": {
        "volumes": [
          {
            "name": "default-token-wl2zc",
            "secret": {
              "secretName": "default-token-wl2zc",
              "defaultMode": 420
            }
          }
        ],
        "containers": [
          {
            "name": "skipper-ingress",
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "args": [
              "skipper",
              "-kubernetes",
              "-kubernetes-in-cluster",
              "-address=:9999",
              "-proxy-preserve-host",
              "-serve-host-metrics",
              "-enable-ratelimits",
              "-experimental-upgrade",
              "-metrics-exp-decay-sample"
            ],
            "ports": [
              {
                "name": "ingress-port",
                "hostPort": 9999,
                "containerPort": 9999,
                "protocol": "TCP"
              }
            ],
            "resources": {
              "limits": {
                "cpu": "200m",
                "memory": "200Mi"
              },
              "requests": {
                "cpu": "25m",
                "memory": "25Mi"
              }
            },
            "volumeMounts": [
              {
                "name": "default-token-wl2zc",
                "readOnly": true,
                "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount"
              }
            ],
            "readinessProbe": {
              "httpGet": {
                "path": "/kube-system/healthz",
                "port": 9999,
                "scheme": "HTTP"
              },
              "initialDelaySeconds": 5,
              "timeoutSeconds": 5,
              "periodSeconds": 10,
              "successThreshold": 1,
              "failureThreshold": 3
            },
            "terminationMessagePath": "/dev/termination-log",
            "terminationMessagePolicy": "File",
            "imagePullPolicy": "IfNotPresent",
            "securityContext": {
              "privileged": false
            }
          }
        ],
        "restartPolicy": "Always",
        "terminationGracePeriodSeconds": 30,
        "dnsPolicy": "ClusterFirst",
        "serviceAccountName": "default",
        "serviceAccount": "default",
        "nodeName": "ip-172-31-21-229.eu-central-1.compute.internal",
        "hostNetwork": true,
        "securityContext": {},
        "imagePullSecrets": [
          {
            "name": "pierone.stups.zalan.do"
          }
        ],
        "affinity": {
          "nodeAffinity": {
            "requiredDuringSchedulingIgnoredDuringExecution": {
              "nodeSelectorTerms": [
                {
                  "matchExpressions": [
                    {
                      "key": "master",
                      "operator": "DoesNotExist"
                    }
                  ]
                }
              ]
            }
          }
        },
        "schedulerName": "default-scheduler",
        "tolerations": [
          {
            "key": "CriticalAddonsOnly",
            "operator": "Exists"
          },
          {
            "key": "node.alpha.kubernetes.io/notReady",
            "operator": "Exists",
            "effect": "NoExecute"
          },
          {
            "key": "node.alpha.kubernetes.io/unreachable",
            "operator": "Exists",
            "effect": "NoExecute"
          }
        ]
      },
      "status": {
        "phase": "Running",
        "conditions": [
          {
            "type": "Initialized",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:07:54Z"
          },
          {
            "type": "Ready",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:08:24Z"
          },
          {
            "type": "PodScheduled",
            "status": "True",
            "lastProbeTime": null,
            "lastTransitionTime": "2017-12-13T20:08:16Z"
          }
        ],
        "hostIP": "172.31.21.229",
        "podIP": "127.0.0.1",
        "startTime": "2017-12-13T20:07:54Z",
        "containerStatuses": [
          {
            "name": "skipper-ingress",
            "state": {
              "running": {
                "startedAt": "2017-12-13T20:08:16Z"
              }
            },
            "lastState": {},
            "ready": true,
            "restartCount": 0,
            "image": "registry.opensource.zalan.do/pathfinder/skipper:v0.9.120",
            "imageID": "docker-pullable://registry.opensource.zalan.do/pathfinder/skipper@sha256:65e409ae5b2d553181bd086cc59aaaaa19c9b5b6e9aadd3c7b8122606f03744a",
            "containerID": "docker://103d466d1b12b96c139beef19bfe42070651200bc6a4144c2b27ab378885aaf3"
          }
        ],
        "qosClass": "Burstable"
      }
    }
  ]
}
	`
