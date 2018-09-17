package apimonitoring

import (
	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

const (
	name = "apimonitoring"
)

var (
	log = logrus.WithField("filter", name)
)

func New() filters.Spec {
	return &apiMonitoringFilterSpec{}
}
