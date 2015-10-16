package flowid

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
)

const (
	defaultLen = 16
	maxLength  = 254
	minLength  = 8
)

var (
	ErrInvalidLen = errors.New(fmt.Sprintf("Invalid length. len must be >= %d and < %d", minLength, maxLength))
	flowIdRegex   = regexp.MustCompile(`^[\w+/=\-]+$`)
)

func newFlowId(len uint8) (string, error) {
	if len < minLength || len > maxLength || len%2 != 0 {
		return "", ErrInvalidLen
	}

	u := make([]byte, hex.DecodedLen(int(len)))
	buf := make([]byte, len)

	rand.Read(u)
	hex.Encode(buf, u)
	return string(buf), nil
}

func isValid(flowId string) bool {
	return len(flowId) >= minLength && len(flowId) <= maxLength && flowIdRegex.MatchString(flowId)
}
