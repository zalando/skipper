package definitions

import "fmt"

type ingressSpec struct {
	DefaultBackend *backend `json:"backend"`
	Rules          []*rule  `json:"rules"`
}

type backend struct {
	ServiceName string      `json:"serviceName"`
	ServicePort backendPort `json:"servicePort"`
	// Traffic field used for custom traffic weights, but not part of the ingress spec.
	Traffic float64
	// number of True predicates to add to support multi color traffic switching
	noopCount int
}

type rule struct {
	Host string    `json:"host"`
	Http *httpRule `json:"http"`
}

type backendPort struct {
	value interface{}
}

type httpRule struct {
	Paths []*pathRule `json:"paths"`
}

type pathRule struct {
	Path    string   `json:"path"`
	Backend *backend `json:"backend"`
}

func (b backend) String() string {
	return fmt.Sprintf("svc(%s, %s) %0.2f", b.ServiceName, b.ServicePort, b.Traffic)
}
