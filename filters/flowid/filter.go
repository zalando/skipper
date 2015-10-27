package flowid

import (
	"github.com/zalando/skipper/filters"
	"log"
	"strings"
)

const defaultLen = 16

const (
	Name                = "flowId"
	ReuseParameterValue = "reuse"
	HeaderName          = "X-Flow-Id"
)

type flowIdSpec struct{}

type flowId struct {
	reuseExisting bool
	flowIdLength  int
}

func New() *flowIdSpec {
	return &flowIdSpec{}
}

func (f *flowId) Request(fc filters.FilterContext) {
	r := fc.Request()
	var flowId string

	if f.reuseExisting {
		flowId = r.Header.Get(HeaderName)
		if isValid(flowId) {
			return
		}
	}

	flowId, err := NewFlowId(f.flowIdLength)
	if err == nil {
		r.Header.Set(HeaderName, flowId)
	} else {
		log.Println(err)
	}
}

func (f *flowId) Response(filters.FilterContext) {}

func (spec *flowIdSpec) CreateFilter(fc []interface{}) (filters.Filter, error) {
	var reuseExisting bool
	if len(fc) > 0 {
		if r, ok := fc[0].(string); ok {
			reuseExisting = strings.ToLower(r) == ReuseParameterValue
		} else {
			return nil, filters.ErrInvalidFilterParameters
		}
	}
	var flowIdLength = defaultLen
	if len(fc) > 1 {
		if l, ok := fc[1].(float64); ok && l >= MinLength && l <= MaxLength {
			flowIdLength = int(l)
		} else {
			return nil, filters.ErrInvalidFilterParameters
		}
	}
	return &flowId{reuseExisting, flowIdLength}, nil
}

func (spec *flowIdSpec) Name() string { return Name }
