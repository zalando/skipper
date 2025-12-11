package flowid

import (
	"fmt"
	"math/rand/v2"
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
	ErrInvalidLen       = fmt.Errorf("invalid length, must be between %d and %d", MinLength, MaxLength)
	standardFlowIDRegex = regexp.MustCompile(`^[0-9a-zA-Z+-]+$`)
)

type standardGenerator struct {
	length int
}

// NewStandardGenerator creates a new FlowID generator that generates flow IDs with length l.
// The alphabet is limited to 64 elements and requires a random 6 bit value to index any of them.
// The cost to rnd.IntXX is not very relevant but the bit shifting operations are faster.
// For this reason a single call to rnd.Int63 is used and its bits are mapped up to 10 chunks of 6 bits each.
// The byte data type carries 2 additional bits for the next chunk which are cleared with the alphabet bit mask.
// It is safe for concurrent use.
func NewStandardGenerator(l int) (Generator, error) {
	if l < MinLength || l > MaxLength {
		return nil, ErrInvalidLen
	}

	return &standardGenerator{length: l}, nil
}

// Generate returns a new Flow ID from the built-in generator with the configured length
func (g *standardGenerator) Generate() (string, error) {
	u := make([]byte, g.length)
	for i := 0; i < g.length; i += 10 {
		b := rand.Int64() // #nosec
		for e := 0; e < 10 && i+e < g.length; e++ {
			c := byte(b>>uint(6*e)) & alphabetBitMask // 6 bits only
			u[i+e] = flowIdAlphabet[c]
		}
	}

	return string(u), nil
}

// MustGenerate is a convenience function equivalent to Generate that panics on failure instead of returning an error.
func (g *standardGenerator) MustGenerate() string {
	id, err := g.Generate()
	if err != nil {
		panic(err)
	}
	return id
}

// IsValid checks if the given flowId follows the format of this generator
func (g *standardGenerator) IsValid(flowId string) bool {
	return len(flowId) >= MinLength && len(flowId) <= MaxLength && standardFlowIDRegex.MatchString(flowId)
}
