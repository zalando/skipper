package main

import (
	"github.com/zalando/skipper/filters"
	"log/slog"
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
	slogHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	logger := slog.New(slogHandler)

	filter := &attestationFilter{
		repo:       NewRepo(os.Getenv("DYNAMO_TABLE_NAME")),
		googlePlay: newGooglePlayIntegrityServiceClient(logger),
		appStore:   newAppStoreIntegrityServiceClient(logger),
		logger:     logger,
	}

	return filter, nil
}
