package flowid

import (
	"github.com/zalando/skipper/skipper"
	"strings"
	"log"
)

const (
	filterName          = "flowId"
	flowIdHeaderName    = "X-Flow-Id"
	defaultLen          = 16
	reuseParameterValue = "reuse"
)

type flowIdSpec struct{}

type flowId struct {
	id            string
	reuseExisting bool
	flowIdLength  int
}

func New() skipper.FilterSpec {
	return &flowIdSpec{}
}

func (this *flowId) Id() string { return this.id }

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
	} else {
		log.Println(err)
	}
}

func (this *flowId) Response(skipper.FilterContext) {}

func (this *flowIdSpec) MakeFilter(id string, fc skipper.FilterConfig) (skipper.Filter, error) {
	var reuseExisting bool
	if len(fc) > 0 {
		if r, ok := fc[0].(string); ok {
			reuseExisting = strings.ToLower(r) == reuseParameterValue
		} else {
			return nil, skipper.ErrInvalidFilterParameters
		}
	}
	var flowIdLength = defaultLen
	if len(fc) > 1 {
		if l, ok := fc[1].(float64); ok && l >= minLength && l <= maxLength {
			flowIdLength = int(l)
		} else {
			return nil, skipper.ErrInvalidFilterParameters
		}
	}
	return &flowId{id, reuseExisting, flowIdLength}, nil
}

func (this *flowIdSpec) Name() string { return filterName }
