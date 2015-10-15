package flowid

import (
	"errors"
	"github.com/zalando/skipper/skipper"
)

const (
	filterName       = "FlowId"
	flowIdHeaderName = "X-Flow-Id"
)

type flowId struct {
	id            string
	reuseExisting bool
	flowIdLength  uint8
}

var (
	ErrInvalidFilterParameters = errors.New("Invalid filter parameters")
)

func New(id string, allowOverride bool, len uint8) skipper.Filter {
	return &flowId{id, allowOverride, len}
}

func (this *flowId) Id() string { return this.id }

func (this *flowId) Name() string { return filterName }

func (this *flowId) Request(fc skipper.FilterContext) {
	r := fc.Request()
	var flowId string

	if this.reuseExisting {
		flowId = r.Header.Get(flowIdHeaderName)
		if isValid(flowId) {
			return
		}
	}

	flowId, err := newFlowId(this.flowIdLength)
	if err == nil {
		fc.Request().Header.Set(flowIdHeaderName, flowId)
	}
}

func (this *flowId) Response(skipper.FilterContext) {}

func (this *flowId) MakeFilter(id string, fc skipper.FilterConfig) (skipper.Filter, error) {
	var reuseExisting bool
	if len(fc) > 0 {
		if r, ok := fc[0].(bool); ok {
			reuseExisting = r
		} else {
			return nil, ErrInvalidFilterParameters
		}
	}
	var flowIdLength uint8 = defaultLen
	if len(fc) > 1 {
		if l, ok := fc[1].(float64); ok && l >= minLength && l <= maxLength {
			flowIdLength = uint8(l)
		} else {
			return nil, ErrInvalidFilterParameters
		}
	}
	return New(id, reuseExisting, flowIdLength), nil
}
