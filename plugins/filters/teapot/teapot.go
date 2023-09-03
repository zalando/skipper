package main

import (
	"time"

	"github.com/zalando/skipper/filters"
)

var _ filters.Spec = (*teapotSpec)(nil)

type teapotSpec struct{}

type teapotRoute []struct {
	URI     string `json:"uri"`
	Note    string `json:"note,omitempty"`
	IsRegex bool   `json:"regex,omitempty"`
}

type teapotService struct {
	Name   string      `json:"name"`
	Routes teapotRoute `json:"routes"`
}

type teapotConfig struct {
	Enabled         bool              `json:"enabled"`
	Services        []string          `json:"services"`
	IgnoreCountries []string          `json:"ignoreCountries"`
	OnlyCountries   []string          `json:"onlyCountries"`
	Title           map[string]string `json:"title"`
	Message         map[string]string `json:"message"`
	EndsAt          time.Time         `json:"endsAt"`
	ExtendBy        int               `json:"extendBy"`
}

type teapotError struct {
	Status int            `json:"status"`
	Error  teapotResponse `json:"error"`
}

type teapotResponse struct {
	Type                        int     `json:"type"`
	Title                       *string `json:"title"`
	Message                     *string `json:"message"`
	PredictedUptimeTimestampUTC string  `json:"predictedUptimeTimestampUTC"`
	Global                      bool    `json:"global"`
}

// InitFilter is called by Skipper to create a new instance of the filter when loaded as a plugin
func InitFilter(_ []string) (filters.Spec, error) {
	return &teapotSpec{}, nil
}

func (s *teapotSpec) Name() string {
	return "teapot"
}

func (s *teapotSpec) CreateFilter(_ []interface{}) (filters.Filter, error) {
	teapotFilter := &teapotFilter{}
	teapotFilter.loadServices()
	teapotFilter.loadTeapots()

	return teapotFilter, nil
}
