// Package nitro provides TLS certificate discovery and private key signing
// offloaded to an AWS Nitro Enclave running AWS ACM for Nitro Enclaves
// (p11-kit RPC over AF_VSOCK or AF_UNIX).
//
// Full functionality is enabled when building with the "awsnitro" build tag.
// Without the build tag, New returns ErrNotSupported so default builds remain
// unaffected.
package nitro
