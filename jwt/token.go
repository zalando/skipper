package jwt

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

var (
	errInvalidToken = errors.New("invalid jwt token")
)

type Token struct {
	Claims map[string]interface{}
}

func Parse(value string) (*Token, error) {
	parts := strings.SplitN(value, ".", 4)
	if len(parts) != 3 {
		return nil, errInvalidToken
	}

	var token Token
	err := unmarshalBase64JSON(parts[1], &token.Claims)
	if err != nil {
		return nil, errInvalidToken
	}

	return &token, nil
}

func unmarshalBase64JSON(s string, v interface{}) error {
	d, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return json.Unmarshal(d, v)
}
