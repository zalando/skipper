package secrets

import "net/url"

// SecretsReader is able to get a secret
type SecretsReader interface {
	// GetSecret finds secret by name and returns secret and if found or not
	GetSecret(string) ([]byte, bool)
	// Close should be used on teardown to cleanup a refresher
	// goroutine. Implementers should check of this interface
	// should check nil pointer, such that caller do not need to
	// check.
	Close()
}

// StaticSecret implements SecretsReader interface. Example:
//
//    sec := []byte("mysecret")
//    sss := StaticSecret(sec)
//    b,_ := sss.GetSecret("")
//    string(b) == sec // true
type StaticSecret []byte

// GetSecret returns the static secret
func (st StaticSecret) GetSecret(string) ([]byte, bool) {
	return st, true
}

// Close implements SecretsReader.
func (st StaticSecret) Close() {}

// StaticDelegateSecret delegates with a static string to the wrapped
// SecretsReader
type StaticDelegateSecret struct {
	sr  SecretsReader
	key string
}

// NewStaticDelegateSecret creates a wrapped SecretsReader,
// that use given s to the underlying SecretsReader to return
// the secret.
func NewStaticDelegateSecret(sr SecretsReader, s string) *StaticDelegateSecret {
	return &StaticDelegateSecret{
		sr:  sr,
		key: s,
	}
}

// GetSecret returns the secret looked up by the static key via
// delegated SecretsReader.
func (sds *StaticDelegateSecret) GetSecret(string) ([]byte, bool) {
	return sds.sr.GetSecret(sds.key)
}

// Close delegates to the wrapped SecretsReader.
func (sds *StaticDelegateSecret) Close() {
	sds.sr.Close()
}

// HostSecret can be used to get secrets by hostnames.
type HostSecret struct {
	sr     SecretsReader
	secMap map[string]string
}

// NewHostSecret create a SecretsReader that returns a secret for
// given host. The given map is used to map hostname to the secrets
// reader key to read the secret from.
func NewHostSecret(sr SecretsReader, h map[string]string) *HostSecret {
	return &HostSecret{
		sr:     sr,
		secMap: h,
	}
}

// GetSecret returns secret for given URL string using the hostname.
func (hs *HostSecret) GetSecret(s string) ([]byte, bool) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, false
	}
	hostname := u.Hostname()
	k, ok := hs.secMap[hostname]
	if !ok {
		return nil, false
	}
	b, ok := hs.sr.GetSecret(k)
	if !ok {
		return nil, false
	}
	return b, true
}

func (hs *HostSecret) Close() {
	hs.sr.Close()
}
