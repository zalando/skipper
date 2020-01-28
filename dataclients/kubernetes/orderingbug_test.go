package kubernetes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/skipper/eskip"
)

// the filter and predicate annotations are in the first ingress with the same host
const ingressDocSucceed = `{
	"items": [{
		"metadata": {
			"namespace": "default",
			"name": "sszuecs-demo-new",
			"annotations": {
				"zalando.org/skipper-filter": "ratelimit(2, \"1m\")",
				"zalando.org/skipper-predicate": "Cookie(\"new\", /^y$/)"
			}
		},
		"spec": {
			"rules": [{
				"host": "sszuecs-demo.playground.zalan.do",
				"http": {
					"paths": [{
						"backend": {
							"serviceName": "sszuecs-demo-v2",
							"servicePort": 80
						}
					}]
				}
			}]
		}
	}, {
		"metadata": {
			"namespace": "default",
			"name": "sszuecs-demo-v1"
		},
		"spec": {
			"rules": [{
				"host": "sszuecs-demo.playground.zalan.do",
				"http": {
					"paths": [{
						"backend": {
							"serviceName": "sszuecs-demo-v1",
							"servicePort": 80
						}
					}]
				}
			}]
		}
	}]
}`

// the filter and predicate annotations are in the second ingress with the same host
const ingressDocFail = `{
	"items": [{
		"metadata": {
			"namespace": "default",
			"name": "sszuecs-demo-v1"
		},
		"spec": {
			"rules": [{
				"host": "sszuecs-demo.playground.zalan.do",
				"http": {
					"paths": [{
						"backend": {
							"serviceName": "sszuecs-demo-v1",
							"servicePort": 80
						}
					}]
				}
			}]
		}
	}, {
		"metadata": {
			"namespace": "default",
			"name": "sszuecs-demo-new",
			"annotations": {
				"zalando.org/skipper-filter": "ratelimit(2, \"1m\")",
				"zalando.org/skipper-predicate": "Cookie(\"new\", /^y$/)"
			}
		},
		"spec": {
			"rules": [{
				"host": "sszuecs-demo.playground.zalan.do",
				"http": {
					"paths": [{
						"backend": {
							"serviceName": "sszuecs-demo-v2",
							"servicePort": 80
						}
					}]
				}
			}]
		}
	}]
}`

const servicesDoc = `{
  "items": [
    {
      "metadata": {
        "namespace": "default",
        "name": "sszuecs-demo-v1"
      },
      "spec": {
        "clusterIP": "10.0.0.1",
        "ports": [
          {
            "name": "port",
            "port": 80
          }
        ]
      }
    },
    {
      "metadata": {
        "namespace": "default",
        "name": "sszuecs-demo-v2"
      },
      "spec": {
        "clusterIP": "10.0.0.2",
        "ports": [
          {
            "name": "port",
            "port": 80
          }
        ]
      }
    }
  ]
}`

type apiV1 struct {
	ingresses []byte
	services  []byte
}

func (api apiV1) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case ingressesClusterURI:
		w.Write(api.ingresses)
		return
	case servicesClusterURI:
		w.Write(api.services)
		return
	case endpointsClusterURI:
		w.Write([]byte("{}"))
		return
	default:
		w.WriteHeader(http.StatusNotFound)
		w.Write(nil)
	}
}

func findCustomPredicate(r []*eskip.Route) bool {
	for i := range r {
		for range r[i].Predicates {
			return true
		}
	}

	return false
}

func testOrderingBug(t *testing.T, ingress string) {
	s := httptest.NewServer(apiV1{
		ingresses: []byte(ingress),
		services:  []byte(servicesDoc),
	})
	defer s.Close()

	c, err := New(Options{KubernetesURL: s.URL})
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Close()

	r, err := c.LoadAll()
	if err != nil {
		t.Error(err)
		return
	}

	if !findCustomPredicate(r) {
		t.Error("failed to find custom predicate")
	}
}

func TestOrderingBug(t *testing.T) {
	t.Run("succeed", func(t *testing.T) {
		testOrderingBug(t, ingressDocSucceed)
	})

	t.Run("fail :(", func(t *testing.T) {
		testOrderingBug(t, ingressDocFail)
	})
}
