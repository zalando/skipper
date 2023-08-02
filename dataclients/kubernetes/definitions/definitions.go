package definitions

import (
	"strings"
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
	Labels      map[string]string `json:"labels"`
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

// TODO: use https://pkg.go.dev/errors#Join with go1.21
func errorsJoin(errs ...error) error {
	var errVals []string
	for _, err := range errs {
		if err != nil {
			errVals = append(errVals, err.Error())
		}
	}
	if len(errVals) > 0 {
		return errors.New(strings.Join(errVals, "\n"))
	}
	return nil
}
