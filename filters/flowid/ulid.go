package flowid

import (
	"github.com/oklog/ulid"
	"io"
	"math/rand"
	"sync"
	"time"
)

type ulidGenerator struct {
	sync.Mutex
	r io.Reader
}

func NewULIDGenerator() Generator {
	return NewULIDGeneratorWithEntropy(rand.New(rand.NewSource(time.Now().UTC().UnixNano())))
}

func NewULIDGeneratorWithEntropy(r io.Reader) Generator {
	return &ulidGenerator{r: r}
}

func (g *ulidGenerator) Generate() (string, error) {
	g.Lock()
	id, err := ulid.New(ulid.Now(), g.r)
	g.Unlock()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func (g *ulidGenerator) MustGenerate() string {
	flowId, err := g.Generate()
	if err != nil {
		panic(err)
	}
	return flowId
}
