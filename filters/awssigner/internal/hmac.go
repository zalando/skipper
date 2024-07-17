package awssigner

import (
	"crypto/hmac"
	"crypto/sha256"
)

func HMACSHA256(key []byte, data []byte) []byte {
	hash := hmac.New(sha256.New, key)
	hash.Write(data)
	return hash.Sum(nil)
}
