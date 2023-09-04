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
