package nitro

import (
	"errors"
	"io"
	"time"
)

const (
	// DefaultEnclavePort is the default vsock port used by p11-kit server in the ACM Nitro Enclave.
	DefaultEnclavePort = 9999

	// DefaultModulePath is the default p11-kit module configuration path written by nitro-enclaves-acm.
	DefaultModulePath = "/etc/pkcs11/modules/p11ne.module"

	// DefaultConfPath is the default glob pattern for target configuration files written by nitro-enclaves-acm.
	DefaultConfPath = "/etc/pki/skipper/*.conf"

	// DefaultRefreshInterval is the default interval for polling the Nitro Enclave for certificate updates.
	DefaultRefreshInterval = 5 * time.Minute
)

// ErrUnsupported is returned when the binary was built without the awsnitro build tag or on a non-Linux platform.
var ErrUnsupported = errors.New("AWS Nitro Enclave TLS is not supported in this build (requires linux and -tags awsnitro)")

// DialFunc connects to an enclave endpoint identified by CID and port.
type DialFunc func(cid, port uint32) (io.ReadWriteCloser, error)

// Options configures the AWS Nitro Enclave TLS certificate provider.
type Options struct {
	EnclaveCID      uint32
	EnclavePort     uint32
	ModulePath      string
	ConfPath        string
	PIN             string
	RefreshInterval time.Duration
	Dial            DialFunc
}
