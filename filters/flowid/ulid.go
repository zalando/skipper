package flowid

import (
	"io"
	"math/rand/v2"
	"regexp"

	"github.com/oklog/ulid"
)

const (
	flowIDLength = 26
)

type ulidGenerator struct {
	r io.Reader
}

var ulidFlowIDRegex = regexp.MustCompile(`^[0123456789ABCDEFGHJKMNPQRSTVWXYZ]{26}$`)

// NewULIDGenerator returns a flow ID generator that is able to generate Universally Unique Lexicographically
// Sortable Identifier (ULID) flow IDs.
// It uses a shared, pseudo-random source of entropy, seeded with the current timestamp.
// It is safe for concurrent usage.
func NewULIDGenerator() Generator {
	return NewULIDGeneratorWithEntropyProvider(&rand.ChaCha8{})
}

// NewULIDGeneratorWithEntropyProvider behaves like NewULIDGenerator but allows you to specify your own source of
// entropy.
// Access to the entropy provider is safe for concurrent usage.
func NewULIDGeneratorWithEntropyProvider(r io.Reader) Generator {
	return &ulidGenerator{r: r}
}

// Generate returns a random ULID flow ID or an empty string in case of failure. The returned error can be inspected
// to assess the failure reason
func (g *ulidGenerator) Generate() (string, error) {
	id, err := ulid.New(ulid.Now(), g.r)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// MustGenerate behaves like Generate but panics in case of failure
func (g *ulidGenerator) MustGenerate() string {
	flowId, err := g.Generate()
	if err != nil {
		panic(err)
	}
	return flowId
}

// IsValid checks if the given flowId follows the format of this generator
func (g *ulidGenerator) IsValid(flowId string) bool {
	return len(flowId) == flowIDLength && ulidFlowIDRegex.MatchString(flowId)
}
