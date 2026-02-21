package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"slices"

	"github.com/zalando/skipper/predicates"
	"github.com/zalando/skipper/routing"
)

type headerSha256Spec struct{}

type headerSha256Predicate struct {
	name   string
	hashes [][sha256.Size]byte
}

// NewHeaderSHA256 creates a predicate specification, whose instances match SHA-256 hash of the header value.
// The HeaderSHA256 predicate requires the header name and one or more hex-encoded SHA-256 hash values of the matching header.
func NewHeaderSHA256() routing.PredicateSpec {
	return &headerSha256Spec{}
}

func (*headerSha256Spec) Name() string {
	return predicates.HeaderSHA256Name
}

// Create a predicate instance matching SHA256 hash of the header value
func (*headerSha256Spec) Create(args []any) (routing.Predicate, error) {
	if len(args) < 2 {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	name, ok := args[0].(string)
	if !ok {
		return nil, predicates.ErrInvalidPredicateParameters
	}

	args = args[1:]

	hashes := make([][sha256.Size]byte, 0, len(args))
	for _, arg := range args {
		hexHash, ok := arg.(string)
		if !ok {
			return nil, predicates.ErrInvalidPredicateParameters
		}
		hash, err := hex.DecodeString(hexHash)
		if err != nil {
			return nil, err
		}
		if len(hash) != sha256.Size {
			return nil, predicates.ErrInvalidPredicateParameters
		}
		hashes = append(hashes, ([sha256.Size]byte)(hash))
	}

	return &headerSha256Predicate{name, hashes}, nil
}

func (p *headerSha256Predicate) Match(r *http.Request) bool {
	value := r.Header.Get(p.name)
	if value == "" {
		return false
	}

	valueHash := sha256.Sum256([]byte(value))

	return slices.Contains(p.hashes, valueHash)
}
