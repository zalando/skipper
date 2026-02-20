package definitions

import (
	"time"

	"errors"
)

var errInvalidMetadata = errors.New("invalid metadata")

type Metadata struct {
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	ResourceVersion string            `json:"resourceVersion,omitempty"`
	Created         time.Time         `json:"creationTimestamp"`
	Uid             string            `json:"uid"`
	Annotations     map[string]string `json:"annotations"`
	Labels          map[string]string `json:"labels"`
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

type WeightedBackend interface {
	GetName() string
	GetWeight() float64
}
