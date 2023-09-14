package main

import (
	"github.com/zalando/skipper/filters"
	"os"
)

var _ filters.Spec = (*attestationSpec)(nil)

type attestationSpec struct{}

// InitFilter is called by Skipper to create a new instance of the filter when loaded as a plugin
func InitFilter(_ []string) (filters.Spec, error) {
	return &attestationSpec{}, nil
}

func (s *attestationSpec) Name() string {
	return "attestation"
}

func (s *attestationSpec) CreateFilter(_ []interface{}) (filters.Filter, error) {
	filter := &attestationFilter{
		repo:       NewRepo(os.Getenv("DYNAMO_TABLE_NAME")),
		googlePlay: newGooglePlayIntegrityServiceClient(),
		appStore:   newAppStoreIntegrityServiceClient(),
	}
	return filter, nil
}
