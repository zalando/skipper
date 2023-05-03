package swarm

import "github.com/zalando/skipper/dataclients/kubernetes"

const (
	// defaultNamespace is used  to find other peer endpoints
	defaultNamespace = "kube-system"
	// defaultName is used to find other peer endpoints
	defaultName = "skipper-ingress"
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
