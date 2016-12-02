package kube

type metadata struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type backend struct {
	ServiceName string `json:"serviceName"`
	ServicePort int    `json:"servicePort"`
}

type pathRule struct {
	Path    string   `json:"path"`
	Backend *backend `json:"backend"`
}

type httpRule struct {
	Paths []*pathRule `json:"paths"`
}

type rule struct {
	Host string    `json:"host"`
	Http *httpRule `json:"http"`
}

type ingressSpec struct {
	DefaultBackend *backend `json:"backend"`
	Rules          []*rule  `json:"rules"`
}

type ingressItem struct {
	Metadata *metadata    `json:"metadata"`
	Spec     *ingressSpec `json:"spec"`
}

type ingressList struct {
	Items []*ingressItem `json:"items"`
}

type serviceSpec struct {
	ClusterIP string `json:"clusterIP"`
}

type service struct {
	Spec *serviceSpec `json:"spec"`
}

type serviceList struct {
	Items []*service `json:"items"`
}
