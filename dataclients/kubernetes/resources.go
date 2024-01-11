package kubernetes

import "github.com/zalando/skipper/dataclients/kubernetes/definitions"

func newResourceID(namespace, name string) definitions.ResourceID {
	return definitions.ResourceID{Namespace: namespace, Name: name}
}

type ClusterResource struct {
	Name string `json:"name"`
}

type ClusterResourceList struct {
	// Items, aka "resources".
	Items []*ClusterResource `json:"resources"`
}

type secret struct {
	Metadata *definitions.Metadata `json:"metadata"`
	Type     string                `json:"type"`
	Data     map[string]string     `json:"data"`
}

type secretList struct {
	Items []*secret `json:"items"`
}

type objectReference struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Uid       string `json:"uid"`
}

func (r *objectReference) getPodName() string {
	if r != nil && r.Kind == "Pod" {
		return r.Name
	}
	return ""
}
