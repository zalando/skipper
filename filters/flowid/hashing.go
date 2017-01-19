package flowid

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"
)

const (
	flowIdAlphabet  = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-+"
	alphabetBitMask = 63
	MaxLength       = 64
	MinLength       = 8
	defaultLen      = 16
)

var (
	ErrInvalidLen = errors.New(fmt.Sprintf("Invalid length. Must be between %d and %d", MinLength, MaxLength))
	flowIdRegex   = regexp.MustCompile(`^[0-9a-zA-Z+-]+$`)
)

// NewFlowId creates a new built-in generator with the defined length and returns a flowid
// This exported function is deprecated and the new flowIdGenerator interface should be used
func NewFlowId(l int) (string, error) {
	g, err := newBuiltInGenerator(l)
	if err != nil {
		return "", fmt.Errorf("deprecated new flowid: %v", err)
	}
	return g.Generate()
}

func isValid(flowId string) bool {
	return len(flowId) >= MinLength && len(flowId) <= MaxLength && flowIdRegex.MatchString(flowId)
}

// builtInGenerator is the default flowID generator
// The alphabet is limited to 64 elements and requires a random 6 bit value to index any of them.
// The cost to rnd.IntXX is not very relevant but the bit shifting operations are faster.
// For this reason a single call to rnd.Int63 is used and its bits are mapped up to 10 chunks of 6 bits each.
// The byte data type carries 2 additional bits for the next chunk which are cleared with the alphabet bit mask.
type builtInGenerator struct {
	length int
}

func newBuiltInGenerator(l int) (flowIDGenerator, error) {
	if l < MinLength || l > MaxLength {
		return nil, ErrInvalidLen
	}

	return &builtInGenerator{length: l}, nil
}

// Generate returns a new Flow ID from the built-in generator with the configured length
func (g *builtInGenerator) Generate() (string, error) {
	u := make([]byte, g.length)
	for i := 0; i < g.length; i += 10 {
		b := rand.Int63()
		for e := 0; e < 10 && i+e < g.length; e++ {
			c := byte(b>>uint(6*e)) & alphabetBitMask // 6 bits only
			u[i+e] = flowIdAlphabet[c]
		}
	}

	return string(u), nil
}

// MustGenerate is a convenience function equivalent to Generate that panics on failure instead of returning an error.
func (g *builtInGenerator) MustGenerate() string {
	id, err := g.Generate()
	if err != nil {
		panic(err)
	}
	return id
}
