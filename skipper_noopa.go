//go:build !opa

package skipper

import (
	"errors"

	ot "github.com/opentracing/opentracing-go"

	"github.com/zalando/skipper/metrics"
)

func newOpenPolicyAgent(o *Options, _ ot.Tracer, _ metrics.Metrics) (*openPolicyAgent, error) {
	if o.EnableOpenPolicyAgent {
		return nil, errors.New("skipper was built without Open Policy Agent support, rebuild with -tags opa")
	}
	return &openPolicyAgent{close: func() {}}, nil
}
