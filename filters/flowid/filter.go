package flowid

import (
	"fmt"
	"log"
	"strings"

	"github.com/zalando/skipper/filters"
)

const (
	// Deprecated, use filters.FlowIdName instead
	Name = filters.FlowIdName

	ReuseParameterValue = "reuse"
	HeaderName          = "X-Flow-Id"
)

// Generator interface should be implemented by types that can generate request tracing Flow IDs.
type Generator interface {
	// Generate returns a new Flow ID using the implementation specific format or an error in case of failure.
	Generate() (string, error)
	// MustGenerate behaves like Generate but panics on failure instead of returning an error.
	MustGenerate() string
	// IsValid asserts if a given flow ID follows an expected format
	IsValid(string) bool
}

type flowIdSpec struct {
	generator Generator
}

type flowId struct {
	reuseExisting bool
	generator     Generator
}

// NewFlowId creates a new standard generator with the defined length and returns a Flow ID.
//
// Deprecated: For backward compatibility this exported function is still available but will be removed in upcoming
// releases. Use the new Generator interface and respective implementations
func NewFlowId(l int) (string, error) {
	g, err := NewStandardGenerator(l)
	if err != nil {
		return "", fmt.Errorf("deprecated new flowid: %w", err)
	}
	return g.Generate()
}

// New creates a new instance of the flowId filter spec which uses the StandardGenerator.
// To use another type of Generator use NewWithGenerator()
func New() *flowIdSpec {
	g, err := NewStandardGenerator(defaultLen)
	if err != nil {
		panic(err)
	}
	return NewWithGenerator(g)
}

// NewWithGenerator behaves like New but allows you to specify any other Generator.
func NewWithGenerator(g Generator) *flowIdSpec {
	return &flowIdSpec{generator: g}
}

// Request will inspect the current Request for the presence of an X-Flow-Id header which will be kept if the
// "reuse" flag has been set. In any other case it will set the same header with the value returned from the
// defined Flow ID Generator
func (f *flowId) Request(fc filters.FilterContext) {
	r := fc.Request()
	var flowId string

	if f.reuseExisting {
		flowId = r.Header.Get(HeaderName)
		if f.generator.IsValid(flowId) {
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

// Response is No-Op in this filter
func (*flowId) Response(filters.FilterContext) {}

// CreateFilter will return a new flowId filter from the spec
// If at least 1 argument is present and it contains the value "reuse", the filter instance is configured to accept
// keep the value of the X-Flow-Id header, if it's already set
func (spec *flowIdSpec) CreateFilter(fc []any) (filters.Filter, error) {
	var reuseExisting bool
	if len(fc) > 0 {
		if r, ok := fc[0].(string); ok {
			reuseExisting = strings.ToLower(r) == ReuseParameterValue
		} else {
			return nil, filters.ErrInvalidFilterParameters
		}
		if len(fc) > 1 {
			log.Println("flow id filter warning: this syntax is deprecated and will be removed soon. " +
				"please check updated docs")
		}
	}
	return &flowId{reuseExisting: reuseExisting, generator: spec.generator}, nil
}

// Name returns the canonical filter name
func (*flowIdSpec) Name() string { return filters.FlowIdName }
