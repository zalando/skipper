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

func New(verbose bool) filters.Spec {
	spec := &apiMonitoringFilterSpec{
		verbose: verbose,
	}
	if verbose {
		log.Infof("Created filter spec: %+v", spec)
	}
	return spec
}
