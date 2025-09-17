package validation

type ResourceType string

const (
	ResourceTypeRouteGroup ResourceType = "RouteGroup"
	ResourceTypeIngress    ResourceType = "Ingress"
)

type Config struct {
	Address  string
	CertFile string
	KeyFile  string
}

type ResourceContext struct {
	Namespace    string
	Name         string
	ResourceType ResourceType
}
