package awssigner

import (
	"strings"
	"sync"
	"time"
)

type derivedKeyCache struct {
	values map[string]derivedKey
	mutex  sync.RWMutex
}

type derivedKey struct {
	AccessKey  string
	Date       time.Time
	Credential []byte
}

// SigningKeyDeriver derives a signing key from a set of credentials
type SigningKeyDeriver struct {
	cache derivedKeyCache
}

func NewSigningKeyDeriver() *SigningKeyDeriver {
	return &SigningKeyDeriver{
		cache: newDerivedKeyCache(),
	}
}

// DeriveKey returns a derived signing key from the given credentials to be used with SigV4 signing.
func (k *SigningKeyDeriver) DeriveKey(credential Credentials, service, region string, signingTime SigningTime) []byte {
	return k.cache.getSigningKey(credential, service, region, signingTime)
}

// copied from https://github.com/aws/aws-sdk-go-v2/blob/v1.25.0/aws/signer/internal/v4/cache.go#L11
func lookupKey(service, region string) string {
	var s strings.Builder
	s.Grow(len(region) + len(service) + 3)
	s.WriteString(region)
	s.WriteRune('/')
	s.WriteString(service)
	return s.String()
}

func (s *derivedKeyCache) get(key string, credentials Credentials, signingTime time.Time) ([]byte, bool) {
	cacheEntry, ok := s.retrieveFromCache(key)
	if ok && cacheEntry.AccessKey == credentials.AccessKeyID && isSameDay(signingTime, cacheEntry.Date) {
		return cacheEntry.Credential, true
	}
	return nil, false
}

func (s *derivedKeyCache) retrieveFromCache(key string) (derivedKey, bool) {
	if v, ok := s.values[key]; ok {
		return v, true
	}
	return derivedKey{}, false
}

func (s *derivedKeyCache) getSigningKey(credentials Credentials, service, region string, signingTime SigningTime) []byte {
	key := lookupKey(service, region)
	s.mutex.RLock()
	if cred, ok := s.get(key, credentials, signingTime.Time); ok {
		s.mutex.RUnlock()
		return cred
	}
	s.mutex.RUnlock()

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if cred, ok := s.get(key, credentials, signingTime.Time); ok {
		return cred
	}
	cred := deriveKey(credentials.SecretAccessKey, service, region, signingTime)
	entry := derivedKey{
		AccessKey:  credentials.AccessKeyID,
		Date:       signingTime.Time,
		Credential: cred,
	}
	s.values[key] = entry
	return cred
}

func deriveKey(secret, service, region string, t SigningTime) []byte {
	hmacDate := HMACSHA256([]byte("AWS4"+secret), []byte(t.ShortTimeFormat()))
	hmacRegion := HMACSHA256(hmacDate, []byte(region))
	hmacService := HMACSHA256(hmacRegion, []byte(service))
	return HMACSHA256(hmacService, []byte("aws4_request"))
}

func newDerivedKeyCache() derivedKeyCache {
	return derivedKeyCache{
		values: make(map[string]derivedKey),
	}
}
