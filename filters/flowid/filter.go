package flowid

import (
	"github.com/zalando/skipper/skipper"
)

const (
	filterName       = "FlowId"
	flowIdHeaderName = "X-Flow-Id"
)

type flowId struct {
	id            string
	reuseExisting bool
}

func New(id string, allowOverride bool) skipper.Filter {
	return &flowId{id, allowOverride}
}

func (this *flowId) Id() string { return this.id }

func (this *flowId) Name() string { return filterName }

func (this *flowId) Request(fc skipper.FilterContext) {
	r := fc.Request()
	var flowId string

	if this.reuseExisting {
		flowId = r.Header.Get(flowIdHeaderName)
	}

	var err error
	if !isValid(flowId) {
		flowId, err = newFlowId(defaultLen)
	}

	if err == nil {
		fc.Request().Header.Set(flowIdHeaderName, flowId)
	}
}

func (this *flowId) Response(skipper.FilterContext) {}

func (this *flowId) MakeFilter(id string, fc skipper.FilterConfig) (skipper.Filter, error) {
	reuseExisting, _ := fc[0].(bool)
	return New(id, reuseExisting), nil
}
