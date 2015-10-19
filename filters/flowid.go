package filters

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"math/rand"
	"regexp"
)

const (
	defaultLen          = 16
	flowIdAlphabet  = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-+"
	alphabetBitMask = 63
)

const (
	FlowIdName          = "flowId"
	FlowIdReuseParameterValue = "reuse"
	FlowIdHeaderName    = "X-Flow-Id"
	FlowIdMaxLength       = 64
	FlowIdMinLength       = 8
)

var (
	ErrInvalidLen = errors.New(fmt.Sprintf("Invalid length. Must be between %d and %d", FlowIdMinLength, FlowIdMaxLength))
	flowIdRegex   = regexp.MustCompile(`^[0-9a-zA-Z+-]+$`)
)

type flowIdSpec struct{}

type flowId struct {
	reuseExisting bool
	flowIdLength  int
}

func NewFlowId() Spec {
	return &flowIdSpec{}
}

func (f *flowId) Request(fc FilterContext) {
	r := fc.Request()
	var flowId string

	if f.reuseExisting {
		flowId = r.Header.Get(FlowIdHeaderName)
		if isValid(flowId) {
			return
		}
	}

	flowId, err := newFlowId(f.flowIdLength)
	if err == nil {
		r.Header.Set(FlowIdHeaderName, flowId)
	} else {
		log.Println(err)
	}
}

func (f *flowId) Response(FilterContext) {}

func (spec *flowIdSpec) CreateFilter(fc []interface{}) (Filter, error) {
	var reuseExisting bool
	if len(fc) > 0 {
		if r, ok := fc[0].(string); ok {
			reuseExisting = strings.ToLower(r) == FlowIdReuseParameterValue
		} else {
			return nil, ErrInvalidFilterParameters
		}
	}
	var flowIdLength = defaultLen
	if len(fc) > 1 {
		if l, ok := fc[1].(float64); ok && l >= FlowIdMinLength && l <= FlowIdMaxLength {
			flowIdLength = int(l)
		} else {
			return nil, ErrInvalidFilterParameters
		}
	}
	return &flowId{reuseExisting, flowIdLength}, nil
}

func (spec *flowIdSpec) Name() string { return FlowIdName }

// newFlowId returns a random flowId using the flowIdAlphabet with length l
// The alphabet is limited to 64 elements and requires a random 6 bit value to index any of them
// The cost to rnd.IntXX is not very relevant but the bit shifting operations are faster
// For this reason a single call to rnd.Int63 is used and its bits are mapped up to 10 chunks of 6 bits each
// The byte data type carries 2 additional bits for the next chunk which are cleared with the alphabet bit mask
func newFlowId(l int) (string, error) {
	if l < FlowIdMinLength || l > FlowIdMaxLength {
		return "", ErrInvalidLen
	}

	u := make([]byte, l)
	for i := 0; i < l; i += 10 {
		b := rand.Int63()
		for e := 0; e < 10 && i+e < l; e++ {
			c := byte(b>>uint(6*e)) & alphabetBitMask // 6 bits only
			u[i+e] = flowIdAlphabet[c]
		}
	}

	return string(u), nil
}

func isValid(flowId string) bool {
	return len(flowId) >= FlowIdMinLength && len(flowId) <= FlowIdMaxLength && flowIdRegex.MatchString(flowId)
}
