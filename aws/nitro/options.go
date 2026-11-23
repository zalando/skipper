package nitro

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/zalando/skipper/metrics"
)

// ErrNotSupported is returned if the binary was built without the
// "awsnitro" build tag or on a platform without vsock support.
var ErrNotSupported = errors.New("aws nitro enclave support not compiled in, build with -tags awsnitro on linux")

const (
	// DefaultRefreshInterval is the default interval to check the
	// certificate chain files for changes.
	DefaultRefreshInterval = time.Minute
	// DefaultSessionsPerToken is the default number of concurrent
	// enclave connections per token.
	DefaultSessionsPerToken = 8
	// DefaultTimeout is the default timeout for enclave operations.
	DefaultTimeout = 5 * time.Second
)

// Certificate configures one ACM certificate in the enclave.
type Certificate struct {
	// TokenLabel is the PKCS#11 token label that holds the private key.
	TokenLabel string
	// ChainPath is the PEM encoded certificate chain file, leaf first.
	ChainPath string
	// KeyLabel optionally selects the private key by its label if the
	// token holds more than one private key.
	KeyLabel string
}

// Options for the Provider.
type Options struct {
	// CID is the vsock context identifier of the enclave.
	CID uint32
	// Port is the vsock port of the p11-kit RPC server in the enclave.
	Port uint32
	// PINFile is the path to a file containing the token PIN. If
	// empty, no login is performed.
	PINFile string
	// Certificates to load from the enclave.
	Certificates []Certificate
	// RefreshInterval to check certificate chain files for changes.
	RefreshInterval time.Duration
	// SessionsPerToken is the maximum number of concurrent enclave
	// connections per token.
	SessionsPerToken int
	// Timeout for a single enclave operation.
	Timeout time.Duration
	// Metrics to record enclave operations, defaults to metrics.Default.
	Metrics metrics.Metrics

	//lint:ignore U1000 used with the awsnitro build tag
	dial func(context.Context) (net.Conn, error)
}

// ParseCertificate parses a certificate configuration of the form
// "token=<label>,chain=<path>[,key=<label>]".
func ParseCertificate(s string) (Certificate, error) {
	var c Certificate
	for kv := range strings.SplitSeq(s, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(kv), "=")
		if !ok || v == "" {
			return c, fmt.Errorf("invalid aws nitro certificate %q: expected key=value", s)
		}
		switch k {
		case "token":
			c.TokenLabel = v
		case "chain":
			c.ChainPath = v
		case "key":
			c.KeyLabel = v
		default:
			return c, fmt.Errorf("invalid aws nitro certificate %q: unknown key %q", s, k)
		}
	}
	if c.TokenLabel == "" || c.ChainPath == "" {
		return c, fmt.Errorf("invalid aws nitro certificate %q: token and chain are required", s)
	}
	return c, nil
}

// String returns the certificate configuration in the format accepted
// by ParseCertificate.
func (c Certificate) String() string {
	s := "token=" + c.TokenLabel + ",chain=" + c.ChainPath
	if c.KeyLabel != "" {
		s += ",key=" + c.KeyLabel
	}
	return s
}

// ErrNoSNI is returned by the GetCertificate function created by
// RequireSNI if the client did not send a server name.
var ErrNoSNI = errors.New("tls: client did not send a server name")

// ErrUnknownServerName is returned by the GetCertificate function
// created by RequireSNI if no certificate is found for the server name.
var ErrUnknownServerName = errors.New("tls: no certificate for server name")

// RequireSNI wraps getCertificate such that handshakes without SNI and
// handshakes for unknown server names fail instead of falling back to
// a default certificate.
func RequireSNI(getCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error), m metrics.Metrics) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	if m == nil {
		m = metrics.Default
	}
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		if hello.ServerName == "" {
			m.IncCounter("awsnitro.handshake.rejected.no_sni")
			return nil, ErrNoSNI
		}
		cert, err := getCertificate(hello)
		if err != nil {
			return nil, err
		}
		if cert == nil {
			m.IncCounter("awsnitro.handshake.rejected.unknown_host")
			return nil, fmt.Errorf("%w: %s", ErrUnknownServerName, hello.ServerName)
		}
		return cert, nil
	}
}
