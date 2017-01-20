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

// The Generator interface can be used to define custom request tracing Flow ID implementations.
type Generator interface {

	// Generate returns a new Flow ID using the implementation specific format or an error in case of failure.
	Generate() (string, error)

	// MustGenerate behaves like Generate but panics on failure instead of returning an error.
	MustGenerate() string
}

type flowIdSpec struct{
	name string
	generator Generator
}

type flowId struct {
	reuseExisting bool
	generator     Generator
}

// and here i would still consider if we should have a flowId() and a ulidFlowId() filter with different names
// what do you think?
func New() filters.Spec {
	return &flowIdSpec{}
}

// question: is it fine that this way the custom filters won't have route specific arguments?
func WithGenerator(name string, g Generator) filters.Spec {
	return &flowIdSpec{name: name, generator: g}
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

	if spec.generator != nil {
		return &flowId{reuseExisting: reuseExisting, generator: spec.generator}, nil
	}

	var (
		gen Generator
		err error
	)
	gen, err = NewStandardGenerator(defaultLen)
	if len(fc) > 1 {
		switch val := fc[1].(type) {
		case float64:
			gen, err = NewStandardGenerator(int(val))
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

func (spec *flowIdSpec) Name() string {
	if spec.name == "" {
		return Name
	}

	return spec.name
}

func createGenerator(generatorId string) (Generator, error) {
	switch generatorId {
	case "ulid":
		return NewULIDGenerator(), nil
	default:
		return NewStandardGenerator(defaultLen)
	}
}
