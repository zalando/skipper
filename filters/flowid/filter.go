package flowid

import (
	"github.com/zalando/skipper/filters"
	"log"
	"strings"
)

const (
	Name                = "flowId"
	ReuseParameterValue = "reuse"
	HeaderName          = "X-Flow-Id"
)

type flowIdSpec struct{}

type flowId struct {
	reuseExisting bool
	generator     flowIDGenerator
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

	flowId, err := f.generator.Generate()
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
	var (
		gen flowIDGenerator
		err error
	)
	gen, err = newBuiltInGenerator(defaultLen)
	if len(fc) > 1 {
		switch val := fc[1].(type) {
		case float64:
			gen, err = newBuiltInGenerator(int(val))
		case string:
			gen, err = createGenerator(strings.ToLower(val))
		default:
			err = filters.ErrInvalidFilterParameters
		}
	}
	if err != nil {
		return nil, filters.ErrInvalidFilterParameters
	}
	return &flowId{reuseExisting: reuseExisting, generator: gen}, nil
}

func (spec *flowIdSpec) Name() string { return Name }

func createGenerator(generatorId string) (flowIDGenerator, error) {
	switch generatorId {
	case "ulid":
		return newULIDGenerator(), nil
	default:
		return newBuiltInGenerator(defaultLen)
	}
}
