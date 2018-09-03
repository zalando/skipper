package monitoring

import (
	"github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

const (
	name = "monitor"
)

var (
	log = logrus.WithField("filter", "monitoring")
)

func New(foo string) filters.Spec {
	log.Infof("Create new filter spec with `foo` %q", foo)
	return &monitoringSpec{
		Foo: foo,
	}
}
