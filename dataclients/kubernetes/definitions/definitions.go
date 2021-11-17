package definitions

import (
	"time"

	"errors"
)

var errInvalidMetadata = errors.New("invalid metadata")

type Metadata struct {
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Created     time.Time         `json:"creationTimestamp"`
	Uid         string            `json:"uid"`
	Annotations map[string]string `json:"annotations"`
}

type IngressPathRule interface {
	GetPath() string
	GetPathType() string
	GetBackend() IngressBackend
}

type IngressBackend interface {
	GetServiceName() string
	GetServicePort() string
	GetTraffic() *IngressBackendTraffic
}

type IngressBackendTraffic struct {
	Weight float64
	// number of True predicates to add to support multi color traffic switching
	NoopCount int
}

func (meta *Metadata) ToResourceID() ResourceID {
	return ResourceID{
		Namespace: namespaceString(meta.Namespace),
		Name:      meta.Name,
	}
}

func validate(meta *Metadata) error {
	if meta == nil || meta.Name == "" {
		return errInvalidMetadata
	}
	return nil
}

func namespaceString(ns string) string {
	if ns == "" {
		return "default"
	}

	return ns
}
