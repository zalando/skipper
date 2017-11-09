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

// the difference is irrelevant
const serviceDocV1 = `{
	"spec": {
		"clusterIP": "10.0.0.1",
		"ports": [{
			"name": "port",
			"port": 80
		}]
	}
}`

// the difference is irrelevant
const serviceDocV2 = `{
	"spec": {
		"clusterIP": "10.0.0.2",
		"ports": [{
			"name": "port",
			"port": 80
		}]
	}
}`

type api string

func (api api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/namespaces/default/services/sszuecs-demo-v1" {
		w.Write([]byte(serviceDocV1))
		return
	}

	if r.URL.Path == "/api/v1/namespaces/default/services/sszuecs-demo-v2" {
		w.Write([]byte(serviceDocV2))
		return
	}

	w.Write([]byte(api))
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
	s := httptest.NewServer(api(ingress))
	defer s.Close()

	c, err := New(Options{KubernetesURL: s.URL})
	if err != nil {
		t.Error(err)
		return
	}

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
