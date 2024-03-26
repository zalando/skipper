package swarm

import "github.com/zalando/skipper/dataclients/kubernetes"

const (
	defaultName = "skipper-ingress"

	// DefaultNamespace is the default namespace where swarm searches for peer information
	DefaultNamespace = "kube-system"
	// DefaultLabelSelectorKey is *deprecated* and not in use, was the default label key to select Pods for peer information
	DefaultLabelSelectorKey = "application"
	// DefaultLabelSelectorValue  is *deprecated* and not in use, was the default label value to select Pods for peer information
	DefaultLabelSelectorValue = defaultName
)

// KubernetesOptions are Kubernetes specific swarm options, that are
// needed to find peers
type KubernetesOptions struct {
	// KubernetesInCluster is *deprecated* and not in use
	KubernetesInCluster bool
	// KubernetesAPIBaseURL is *deprecated* and not in use
	KubernetesAPIBaseURL string

	// Namespace is the namespace of the Kubernetes endpoint that will be used to detect swarm peers
	Namespace string

	// LabelSelectorKey is *deprecated* and not in use
	LabelSelectorKey string
	// LabelSelectorValue is *deprecated* and not in use
	LabelSelectorValue string

	// Name is the name of the Kubernetes endpoint that will be used to detect swarm peers
	Name string

	// KubernetesClient is used to detect swarm endpoints to join the swarm cluster
	KubernetesClient *kubernetes.Client
}
