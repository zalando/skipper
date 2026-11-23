package nitro

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultRefreshInterval is the default interval for refreshing
	// certificates from the Nitro Enclave.
	DefaultRefreshInterval = 5 * time.Minute

	// DefaultTimeout is the default per-RPC timeout when communicating
	// with the Nitro Enclave.
	DefaultTimeout = 5 * time.Second

	// DefaultMaxSessions is the default maximum number of concurrent
	// PKCS#11 sessions pooled for enclave signing operations.
	DefaultMaxSessions = 8
)

var (
	// ErrNotSupported is returned when AWS Nitro TLS is requested on a binary
	// built without the "awsnitro" build tag or when AF_VSOCK is used on a
	// non-Linux platform.
	ErrNotSupported = errors.New("aws/nitro: binary built without awsnitro build tag")

	// ErrNoCertificates is returned when no usable TLS certificates with
	// matching private keys are found in the enclave token.
	ErrNoCertificates = errors.New("aws/nitro: no certificates found in enclave")
)

// Options configures the AWS Nitro Enclave TLS certificate provider.
type Options struct {
	// Endpoint is the enclave RPC address in URI form:
	// "vsock://<cid>:<port>" or "unix://<path>".
	Endpoint string

	// TokenLabel optionally restricts certificate discovery to the PKCS#11
	// slot whose token label matches TokenLabel.
	TokenLabel string

	// RefreshInterval controls how often certificates are reloaded from the
	// enclave to pick up ACM renewals. Defaults to DefaultRefreshInterval.
	RefreshInterval time.Duration

	// Timeout is the per-RPC I/O deadline for enclave communication.
	// Defaults to DefaultTimeout.
	Timeout time.Duration

	// MaxSessions bounds the number of concurrent PKCS#11 sessions pooled
	// for TLS handshake signing. Defaults to DefaultMaxSessions.
	MaxSessions int
}

// Validate checks Options and applies defaults to zero-valued optional fields.
func (o *Options) Validate() error {
	if o == nil {
		return errors.New("aws/nitro: nil options")
	}
	if _, err := parseEndpoint(o.Endpoint); err != nil {
		return err
	}
	if o.RefreshInterval < 0 {
		return errors.New("aws/nitro: refresh interval must not be negative")
	}
	if o.RefreshInterval == 0 {
		o.RefreshInterval = DefaultRefreshInterval
	}
	if o.Timeout < 0 {
		return errors.New("aws/nitro: timeout must not be negative")
	}
	if o.Timeout == 0 {
		o.Timeout = DefaultTimeout
	}
	if o.MaxSessions < 0 {
		return errors.New("aws/nitro: max sessions must not be negative")
	}
	if o.MaxSessions == 0 {
		o.MaxSessions = DefaultMaxSessions
	}
	return nil
}

type parsedEndpoint struct {
	scheme string
	cid    uint32
	port   uint32
	path   string
}

func parseEndpoint(raw string) (parsedEndpoint, error) {
	if raw == "" {
		return parsedEndpoint{}, errors.New("aws/nitro: endpoint is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return parsedEndpoint{}, fmt.Errorf("aws/nitro: invalid endpoint %q: %w", raw, err)
	}
	switch u.Scheme {
	case "vsock":
		host, portStr, ok := strings.Cut(u.Host, ":")
		if !ok || host == "" || portStr == "" {
			return parsedEndpoint{}, fmt.Errorf("aws/nitro: invalid vsock endpoint %q: expected vsock://<cid>:<port>", raw)
		}
		cid, err := strconv.ParseUint(host, 10, 32)
		if err != nil {
			return parsedEndpoint{}, fmt.Errorf("aws/nitro: invalid vsock cid %q: %w", host, err)
		}
		port, err := strconv.ParseUint(portStr, 10, 32)
		if err != nil || port == 0 {
			return parsedEndpoint{}, fmt.Errorf("aws/nitro: invalid vsock port %q", portStr)
		}
		return parsedEndpoint{
			scheme: "vsock",
			cid:    uint32(cid),
			port:   uint32(port),
		}, nil
	case "unix":
		p := u.Path
		if p == "" {
			return parsedEndpoint{}, fmt.Errorf("aws/nitro: invalid unix endpoint %q: missing socket path", raw)
		}
		return parsedEndpoint{
			scheme: "unix",
			path:   p,
		}, nil
	default:
		return parsedEndpoint{}, fmt.Errorf("aws/nitro: unsupported endpoint scheme %q (expected vsock or unix)", u.Scheme)
	}
}
