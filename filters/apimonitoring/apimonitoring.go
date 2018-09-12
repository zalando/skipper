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

func New(foo string) filters.Spec {
	log.Infof("Create new filter spec with `foo` %q", foo)
	return &apiMonitoringFilterSpec{
		Foo: foo,
	}
}
